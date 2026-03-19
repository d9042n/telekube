// Package watcher provides real-time Kubernetes monitoring with Telegram alerts.
package watcher

import (
	"context"
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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultPVCCheckInterval   = 5 * time.Minute
	defaultPVCWarningPct      = 80
	defaultPVCCriticalPct     = 90
	pvcProgressBarLength      = 10
)

// PVCWatcherConfig holds configuration for the PVC usage watcher.
type PVCWatcherConfig struct {
	CheckInterval     time.Duration
	WarningThreshold  float64 // percentage
	CriticalThreshold float64 // percentage
	Namespaces        []string // empty = all
	ExcludeNamespaces []string
}

// PVCWatcher monitors PersistentVolumeClaim disk usage and alerts on high usage.
type PVCWatcher struct {
	clusters   cluster.Manager
	notifier   Notifier
	audit      audit.Logger
	cfg        config.TelegramConfig
	watcherCfg PVCWatcherConfig
	logger     *zap.Logger

	alertCache map[string]time.Time
	cooldown   time.Duration
	mu         sync.RWMutex
}

// NewPVCWatcher creates a new PVC usage watcher.
func NewPVCWatcher(
	clusters cluster.Manager,
	notifier Notifier,
	auditLogger audit.Logger,
	cfg config.TelegramConfig,
	watcherCfg PVCWatcherConfig,
	logger *zap.Logger,
) *PVCWatcher {
	if watcherCfg.CheckInterval == 0 {
		watcherCfg.CheckInterval = defaultPVCCheckInterval
	}
	if watcherCfg.WarningThreshold == 0 {
		watcherCfg.WarningThreshold = defaultPVCWarningPct
	}
	if watcherCfg.CriticalThreshold == 0 {
		watcherCfg.CriticalThreshold = defaultPVCCriticalPct
	}
	return &PVCWatcher{
		clusters:   clusters,
		notifier:   notifier,
		audit:      auditLogger,
		cfg:        cfg,
		watcherCfg: watcherCfg,
		logger:     logger,
		alertCache: make(map[string]time.Time),
		cooldown:   defaultCooldown,
	}
}

// Start begins polling PVC usage across all clusters.
func (w *PVCWatcher) Start(ctx context.Context) error {
	for _, c := range w.clusters.List() {
		clusterName := c.Name
		go wait.UntilWithContext(ctx, func(ctx context.Context) {
			w.checkCluster(ctx, clusterName)
		}, w.watcherCfg.CheckInterval)
	}
	w.logger.Info("pvc watcher started")
	return nil
}

// checkCluster checks PVC usage for all namespaces in a cluster.
func (w *PVCWatcher) checkCluster(ctx context.Context, clusterName string) {
	cs, err := w.clusters.ClientSet(clusterName)
	if err != nil {
		w.logger.Error("failed to get clientset for pvc watcher",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return
	}

	namespaces := w.watcherCfg.Namespaces
	if len(namespaces) == 0 {
		nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			w.logger.Error("failed to list namespaces for pvc watcher",
				zap.String("cluster", clusterName),
				zap.Error(err),
			)
			return
		}
		for _, ns := range nsList.Items {
			if !w.isPVCExcluded(ns.Name) {
				namespaces = append(namespaces, ns.Name)
			}
		}
	}

	for _, ns := range namespaces {
		if w.isPVCExcluded(ns) {
			continue
		}
		w.checkNamespacePVCs(ctx, cs, clusterName, ns)
	}
}

func (w *PVCWatcher) isPVCExcluded(ns string) bool {
	for _, excl := range w.watcherCfg.ExcludeNamespaces {
		if excl == ns {
			return true
		}
	}
	return false
}

// checkNamespacePVCs checks PVC usage in a namespace using kubelet stats.
func (w *PVCWatcher) checkNamespacePVCs(ctx context.Context, cs kubernetes.Interface, clusterName, namespace string) {
	pvcs, err := cs.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		w.logger.Error("failed to list PVCs",
			zap.String("cluster", clusterName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return
	}

	// Get the actual capacity from PVC capacity status
	for i := range pvcs.Items {
		pvc := &pvcs.Items[i]
		if pvc.Status.Phase != corev1.ClaimBound {
			continue // Only check bound PVCs
		}

		capacity := pvc.Status.Capacity[corev1.ResourceStorage]

		// We can estimate usage from annotations or Prometheus metrics
		// For now, check PVC capacity vs request ratio and look at pod-mounted volumes
		w.checkPVCUsageEstimate(ctx, cs, clusterName, namespace, pvc, capacity)
	}
}

// checkPVCUsageEstimate attempts to estimate PVC usage.
// In production, this would use Prometheus kubelet metrics; here we use annotations
// or resource quotas as fallbacks.
func (w *PVCWatcher) checkPVCUsageEstimate(
	ctx context.Context,
	cs kubernetes.Interface,
	clusterName, namespace string,
	pvc *corev1.PersistentVolumeClaim,
	capacity resource.Quantity,
) {
	// Check if the PVC has usage annotations (set by an external metrics collector)
	usageStr, hasUsage := pvc.Annotations["telekube.io/pvc-usage-bytes"]
	if !hasUsage {
		// No usage annotation — try to get from node stats via pods
		w.checkPVCFromPodMount(ctx, cs, clusterName, namespace, pvc)
		return
	}

	var usedBytes int64
	if _, err := fmt.Sscanf(usageStr, "%d", &usedBytes); err != nil {
		return
	}

	capacityBytes := capacity.Value()
	if capacityBytes == 0 {
		return
	}

	usagePct := float64(usedBytes) / float64(capacityBytes) * 100

	if usagePct >= w.watcherCfg.CriticalThreshold {
		w.alertPVCUsage(clusterName, namespace, pvc.Name, "", usedBytes, capacityBytes, usagePct, SeverityCritical)
	} else if usagePct >= w.watcherCfg.WarningThreshold {
		w.alertPVCUsage(clusterName, namespace, pvc.Name, "", usedBytes, capacityBytes, usagePct, SeverityWarning)
	}
}

// checkPVCFromPodMount tries to find the pod mounting the PVC and estimate disk usage.
func (w *PVCWatcher) checkPVCFromPodMount(
	ctx context.Context,
	cs kubernetes.Interface,
	clusterName, namespace string,
	pvc *corev1.PersistentVolumeClaim,
) {
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == pvc.Name {
				// PVC is mounted by this pod; we can check via kubelet proxy
				// For now we log that we found the mounting pod for future stats collection
				w.logger.Debug("pvc mounted by pod",
					zap.String("cluster", clusterName),
					zap.String("namespace", namespace),
					zap.String("pvc", pvc.Name),
					zap.String("pod", pod.Name),
				)
				return
			}
		}
	}
}

// alertPVCUsage sends a PVC disk usage alert.
func (w *PVCWatcher) alertPVCUsage(clusterName, namespace, pvcName, podName string, usedBytes, capacityBytes int64, usagePct float64, severity AlertSeverity) {
	alertKey := fmt.Sprintf("pvc/%s/%s/%s", clusterName, namespace, pvcName)

	w.mu.RLock()
	lastAlert, exists := w.alertCache[alertKey]
	w.mu.RUnlock()
	if exists && time.Since(lastAlert) < w.cooldown {
		return
	}
	w.mu.Lock()
	w.alertCache[alertKey] = time.Now()
	w.mu.Unlock()

	// Build progress bar
	filled := int(usagePct / 100 * pvcProgressBarLength)
	if filled > pvcProgressBarLength {
		filled = pvcProgressBarLength
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", pvcProgressBarLength-filled)

	usedGiB := float64(usedBytes) / (1024 * 1024 * 1024)
	capGiB := float64(capacityBytes) / (1024 * 1024 * 1024)

	var severityLabel string
	switch severity {
	case SeverityCritical:
		severityLabel = fmt.Sprintf("🔴 CRITICAL: Disk usage at %.0f%%!", usagePct)
	default:
		severityLabel = fmt.Sprintf("⚠️ Warning: Disk usage exceeds %.0f%% threshold!", w.watcherCfg.WarningThreshold)
	}

	var sb strings.Builder
	sb.WriteString("💾 *PVC Alert — High Disk Usage*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString(fmt.Sprintf("Cluster:   %s\n", clusterName))
	sb.WriteString(fmt.Sprintf("Namespace: %s\n", namespace))
	sb.WriteString(fmt.Sprintf("PVC:       `%s`\n", pvcName))
	if podName != "" {
		sb.WriteString(fmt.Sprintf("Pod:       `%s`\n", podName))
	}
	sb.WriteString(fmt.Sprintf("\nUsage:     [%s] %.0f%%\n", bar, usagePct))
	sb.WriteString(fmt.Sprintf("           %.1fGi / %.1fGi\n", usedGiB, capGiB))
	sb.WriteString(fmt.Sprintf("\n%s\n", severityLabel))

	menu := &telebot.ReplyMarkup{}
	pvcData := fmt.Sprintf("%s|%s|%s", pvcName, namespace, clusterName)
	btnDetail := menu.Data("📊 Top Pods", "k8s_top_pods", fmt.Sprintf("%s|%s", namespace, clusterName))
	btnMute := menu.Data("🔇 Mute", "watcher_mute", alertKey)
	menu.Inline(menu.Row(btnDetail, btnMute))

	w.sendPVCAlert(sb.String(), menu)

	w.audit.Log(entity.AuditEntry{
		ID:        ulid.Make().String(),
		UserID:    0,
		Username:  "system:watcher",
		Action:    "alert.pvc.high_usage",
		Resource:  fmt.Sprintf("pvc/%s", pvcName),
		Cluster:   clusterName,
		Namespace: namespace,
		Status:    entity.AuditStatusSuccess,
		Details: map[string]interface{}{
			"severity":       string(severity),
			"usage_pct":     usagePct,
			"used_bytes":    usedBytes,
			"capacity_bytes": capacityBytes,
		},
		OccurredAt: time.Now().UTC(),
	})

	w.logger.Info("pvc usage alert sent",
		zap.String("cluster", clusterName),
		zap.String("namespace", namespace),
		zap.String("pvc", pvcData),
		zap.Float64("usage_pct", usagePct),
		zap.String("severity", string(severity)),
	)
}

func (w *PVCWatcher) sendPVCAlert(text string, markup *telebot.ReplyMarkup) {
	for _, chatID := range w.cfg.AllowedChats {
		if err := w.notifier.SendAlert(chatID, text, markup); err != nil {
			w.logger.Error("failed to send pvc alert to chat",
				zap.Int64("chat_id", chatID),
				zap.Error(err),
			)
		}
	}
	for _, adminID := range w.cfg.AdminIDs {
		if err := w.notifier.SendAlert(adminID, text, markup); err != nil {
			w.logger.Error("failed to send pvc alert to admin",
				zap.Int64("admin_id", adminID),
				zap.Error(err),
			)
		}
	}
}

// AlertPVCUsage is an exported version for external callers (e.g., metrics collectors).
func (w *PVCWatcher) AlertPVCUsage(clusterName, namespace, pvcName, podName string, usedBytes, capacityBytes int64) {
	if capacityBytes == 0 {
		return
	}
	usagePct := float64(usedBytes) / float64(capacityBytes) * 100

	if usagePct >= w.watcherCfg.CriticalThreshold {
		w.alertPVCUsage(clusterName, namespace, pvcName, podName, usedBytes, capacityBytes, usagePct, SeverityCritical)
	} else if usagePct >= w.watcherCfg.WarningThreshold {
		w.alertPVCUsage(clusterName, namespace, pvcName, podName, usedBytes, capacityBytes, usagePct, SeverityWarning)
	}
}
