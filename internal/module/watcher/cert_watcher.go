// Package watcher provides real-time Kubernetes monitoring with Telegram alerts.
package watcher

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultCertCheckInterval = 6 * time.Hour
	defaultAlertDaysBefore   = 30
	defaultCriticalDaysBefore = 7
)

// CertWatcherConfig holds configuration for the certificate expiry watcher.
type CertWatcherConfig struct {
	CheckInterval      time.Duration
	AlertDaysBefore   int      // Warn when cert expires within N days
	CriticalDaysBefore int     // Critical alert when cert expires within N days
	Namespaces         []string // empty = all
	ExcludeNamespaces  []string
}

// CertWatcher monitors TLS certificate secrets and alerts before expiry.
type CertWatcher struct {
	clusters   cluster.Manager
	notifier   Notifier
	audit      audit.Logger
	cfg        config.TelegramConfig
	watcherCfg CertWatcherConfig
	logger     *zap.Logger

	alertCache map[string]time.Time
	cooldown   time.Duration
	mu         sync.RWMutex
}

// NewCertWatcher creates a new certificate expiry watcher.
func NewCertWatcher(
	clusters cluster.Manager,
	notifier Notifier,
	auditLogger audit.Logger,
	cfg config.TelegramConfig,
	watcherCfg CertWatcherConfig,
	logger *zap.Logger,
) *CertWatcher {
	if watcherCfg.CheckInterval == 0 {
		watcherCfg.CheckInterval = defaultCertCheckInterval
	}
	if watcherCfg.AlertDaysBefore == 0 {
		watcherCfg.AlertDaysBefore = defaultAlertDaysBefore
	}
	if watcherCfg.CriticalDaysBefore == 0 {
		watcherCfg.CriticalDaysBefore = defaultCriticalDaysBefore
	}
	return &CertWatcher{
		clusters:   clusters,
		notifier:   notifier,
		audit:      auditLogger,
		cfg:        cfg,
		watcherCfg: watcherCfg,
		logger:     logger,
		alertCache: make(map[string]time.Time),
		cooldown:   defaultCertCheckInterval, // recheck after one interval
	}
}

// Start begins polling certificate expiry across all clusters.
func (w *CertWatcher) Start(ctx context.Context) error {
	for _, c := range w.clusters.List() {
		clusterName := c.Name
		go wait.UntilWithContext(ctx, func(ctx context.Context) {
			w.checkCluster(ctx, clusterName)
		}, w.watcherCfg.CheckInterval)
	}
	w.logger.Info("certificate expiry watcher started")
	return nil
}

// checkCluster checks all TLS secrets in a cluster.
func (w *CertWatcher) checkCluster(ctx context.Context, clusterName string) {
	cs, err := w.clusters.ClientSet(clusterName)
	if err != nil {
		w.logger.Error("failed to get clientset for cert watcher",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return
	}

	namespaces := w.watcherCfg.Namespaces
	if len(namespaces) == 0 {
		nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			w.logger.Error("failed to list namespaces for cert watcher",
				zap.String("cluster", clusterName),
				zap.Error(err),
			)
			return
		}
		for _, ns := range nsList.Items {
			if !w.isCertExcluded(ns.Name) {
				namespaces = append(namespaces, ns.Name)
			}
		}
	}

	for _, ns := range namespaces {
		if w.isCertExcluded(ns) {
			continue
		}
		w.checkNamespace(ctx, cs, clusterName, ns)
	}
}

func (w *CertWatcher) isCertExcluded(ns string) bool {
	for _, excl := range w.watcherCfg.ExcludeNamespaces {
		if excl == ns {
			return true
		}
	}
	return false
}

// checkNamespace checks TLS secrets in a namespace.
func (w *CertWatcher) checkNamespace(ctx context.Context, cs kubernetes.Interface, clusterName, namespace string) {
	secrets, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "type=kubernetes.io/tls",
	})
	if err != nil {
		w.logger.Error("failed to list TLS secrets",
			zap.String("cluster", clusterName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return
	}

	for i := range secrets.Items {
		secret := &secrets.Items[i]
		certPEM, ok := secret.Data["tls.crt"]
		if !ok || len(certPEM) == 0 {
			continue
		}

		block, _ := pem.Decode(certPEM)
		if block == nil {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			w.logger.Warn("failed to parse certificate",
				zap.String("cluster", clusterName),
				zap.String("namespace", namespace),
				zap.String("secret", secret.Name),
				zap.Error(err),
			)
			continue
		}

		daysUntilExpiry := time.Until(cert.NotAfter).Hours() / 24

		if daysUntilExpiry <= float64(w.watcherCfg.AlertDaysBefore) {
			var severity AlertSeverity
			if daysUntilExpiry <= float64(w.watcherCfg.CriticalDaysBefore) {
				severity = SeverityCritical
			} else {
				severity = SeverityWarning
			}
			w.alertCertExpiry(clusterName, namespace, secret.Name, cert, daysUntilExpiry, severity)
		}
	}
}

// alertCertExpiry sends a certificate expiry alert.
func (w *CertWatcher) alertCertExpiry(clusterName, namespace, secretName string, cert *x509.Certificate, daysLeft float64, severity AlertSeverity) {
	alertKey := fmt.Sprintf("cert/%s/%s/%s", clusterName, namespace, secretName)

	w.mu.RLock()
	lastAlert, exists := w.alertCache[alertKey]
	w.mu.RUnlock()
	if exists && time.Since(lastAlert) < w.cooldown {
		return
	}
	w.mu.Lock()
	w.alertCache[alertKey] = time.Now()
	w.mu.Unlock()

	// Collect SANs (Subject Alternative Names)
	domains := append(cert.DNSNames, fmt.Sprintf("%s (CN)", cert.Subject.CommonName))
	domainsStr := strings.Join(domains, ", ")

	issuer := cert.Issuer.CommonName
	if issuer == "" {
		issuer = "Unknown"
	}

	var severityLabel string
	switch severity {
	case SeverityCritical:
		severityLabel = fmt.Sprintf("🔴 CRITICAL: Expires in %.0f days!", daysLeft)
	default:
		severityLabel = fmt.Sprintf("⚠️ Warning: Expires in %.0f days", daysLeft)
	}

	var sb strings.Builder
	sb.WriteString("🔐 *Certificate Expiry Warning*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString(fmt.Sprintf("Cluster:   %s\n", clusterName))
	sb.WriteString(fmt.Sprintf("Namespace: %s\n", namespace))
	sb.WriteString(fmt.Sprintf("Secret:    `%s`\n", secretName))
	if len(domains) > 0 {
		sb.WriteString(fmt.Sprintf("Domains:   %s\n", domainsStr))
	}
	sb.WriteString(fmt.Sprintf("Issuer:    %s\n", issuer))
	sb.WriteString(fmt.Sprintf("Expires:   %s\n", cert.NotAfter.UTC().Format("2006-01-02 15:04 UTC")))
	sb.WriteString(fmt.Sprintf("\n%s\n", severityLabel))

	menu := &telebot.ReplyMarkup{}
	secretData := fmt.Sprintf("%s|%s|%s", secretName, namespace, clusterName)
	btnDetail := menu.Data("📋 Secret Details", "k8s_secret_detail", secretData)
	btnMute := menu.Data("🔇 Mute", "watcher_mute", alertKey)
	menu.Inline(menu.Row(btnDetail, btnMute))

	w.sendCertAlert(sb.String(), menu)

	w.audit.Log(entity.AuditEntry{
		ID:        ulid.Make().String(),
		UserID:    0,
		Username:  "system:watcher",
		Action:    "alert.cert.expiry",
		Resource:  fmt.Sprintf("secret/%s", secretName),
		Cluster:   clusterName,
		Namespace: namespace,
		Status:    entity.AuditStatusSuccess,
		Details: map[string]interface{}{
			"severity":    string(severity),
			"days_left":   daysLeft,
			"expires_at":  cert.NotAfter.UTC().Format(time.RFC3339),
			"domains":     domains,
		},
		OccurredAt: time.Now().UTC(),
	})

	w.logger.Info("certificate expiry alert sent",
		zap.String("cluster", clusterName),
		zap.String("namespace", namespace),
		zap.String("secret", secretName),
		zap.Float64("days_left", daysLeft),
		zap.String("severity", string(severity)),
	)
}

func (w *CertWatcher) sendCertAlert(text string, markup *telebot.ReplyMarkup) {
	for _, chatID := range w.cfg.AllowedChats {
		if err := w.notifier.SendAlert(chatID, text, markup); err != nil {
			w.logger.Error("failed to send cert alert to chat",
				zap.Int64("chat_id", chatID),
				zap.Error(err),
			)
		}
	}
	for _, adminID := range w.cfg.AdminIDs {
		if err := w.notifier.SendAlert(adminID, text, markup); err != nil {
			w.logger.Error("failed to send cert alert to admin",
				zap.Int64("admin_id", adminID),
				zap.Error(err),
			)
		}
	}
}
