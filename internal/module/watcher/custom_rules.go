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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	customRuleEvalInterval = 30 * time.Second
	customRuleCooldown     = 10 * time.Minute
)

// CustomRuleEvaluator evaluates user-defined alert rules from config.
type CustomRuleEvaluator struct {
	rules    []entity.AlertRule
	clusters cluster.Manager
	notifier Notifier
	audit    audit.Logger
	cfg      config.TelegramConfig
	logger   *zap.Logger

	alertCache map[string]time.Time // key → last fired time
	mu         sync.RWMutex
}

// NewCustomRuleEvaluator creates a new evaluator for config-driven alert rules.
func NewCustomRuleEvaluator(
	rules []entity.AlertRule,
	clusters cluster.Manager,
	notifier Notifier,
	auditLogger audit.Logger,
	cfg config.TelegramConfig,
	logger *zap.Logger,
) *CustomRuleEvaluator {
	return &CustomRuleEvaluator{
		rules:      rules,
		clusters:   clusters,
		notifier:   notifier,
		audit:      auditLogger,
		cfg:        cfg,
		logger:     logger,
		alertCache: make(map[string]time.Time),
	}
}

// Start begins the periodic evaluation loop.
func (e *CustomRuleEvaluator) Start(ctx context.Context) error {
	if len(e.rules) == 0 {
		e.logger.Info("no custom alert rules configured, skipping evaluator")
		return nil
	}

	e.logger.Info("custom rule evaluator starting", zap.Int("rules", len(e.rules)))

	go func() {
		ticker := time.NewTicker(customRuleEvalInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				e.logger.Info("custom rule evaluator stopped")
				return
			case <-ticker.C:
				e.evaluate(ctx)
			}
		}
	}()

	return nil
}

// evaluate iterates all rules and checks them against each matching cluster.
func (e *CustomRuleEvaluator) evaluate(ctx context.Context) {
	for _, rule := range e.rules {
		clusterNames := e.resolveClusterScope(rule.Scope.Clusters)
		for _, clusterName := range clusterNames {
			e.evaluateRule(ctx, rule, clusterName)
		}
	}
}

// resolveClusterScope returns the list of clusters to check.
func (e *CustomRuleEvaluator) resolveClusterScope(clusters []string) []string {
	if len(clusters) == 0 || (len(clusters) == 1 && clusters[0] == "*") {
		var names []string
		for _, c := range e.clusters.List() {
			names = append(names, c.Name)
		}
		return names
	}
	return clusters
}

// resolveNamespaceScope returns namespace filter or empty for all.
func resolveNamespaceScope(ns []string) []string {
	if len(ns) == 0 || (len(ns) == 1 && ns[0] == "*") {
		return nil // means all namespaces
	}
	return ns
}

func (e *CustomRuleEvaluator) evaluateRule(ctx context.Context, rule entity.AlertRule, clusterName string) {
	switch rule.Condition.Type {
	case "pod_restart_count":
		e.evalPodRestartCount(ctx, rule, clusterName)
	case "pod_pending_duration":
		e.evalPodPendingDuration(ctx, rule, clusterName)
	case "namespace_quota_percentage":
		e.evalNamespaceQuota(ctx, rule, clusterName)
	case "deployment_unavailable":
		e.evalDeploymentUnavailable(ctx, rule, clusterName)
	default:
		e.logger.Warn("unknown custom rule condition type",
			zap.String("rule", rule.Name),
			zap.String("type", rule.Condition.Type),
		)
	}
}

// ---------- Condition evaluators ----------

func (e *CustomRuleEvaluator) evalPodRestartCount(ctx context.Context, rule entity.AlertRule, clusterName string) {
	namespaces := resolveNamespaceScope(rule.Scope.Namespaces)
	if len(namespaces) > 0 {
		for _, ns := range namespaces {
			e.evalPodRestartCountInNS(ctx, rule, clusterName, ns)
		}
		return
	}
	// All namespaces
	e.evalPodRestartCountInNS(ctx, rule, clusterName, "")
}

func (e *CustomRuleEvaluator) evalPodRestartCountInNS(ctx context.Context, rule entity.AlertRule, clusterName, namespace string) {
	cs, err := e.clusters.ClientSet(clusterName)
	if err != nil {
		return
	}

	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		e.logger.Debug("failed to list pods for custom rule",
			zap.String("rule", rule.Name),
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return
	}

	threshold := rule.Condition.Threshold
	if threshold <= 0 {
		threshold = 5
	}

	for _, pod := range pods.Items {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if int(containerStatus.RestartCount) >= threshold {
				alertKey := fmt.Sprintf("custom:%s:%s/%s/%s", rule.Name, clusterName, pod.Namespace, pod.Name)
				if e.isInCooldown(alertKey) {
					continue
				}
				e.fireAlert(rule, clusterName, pod.Namespace, fmt.Sprintf("pod/%s", pod.Name),
					fmt.Sprintf("Container `%s` restart count: %d (threshold: %d)",
						containerStatus.Name, containerStatus.RestartCount, threshold))
				e.markFired(alertKey)
			}
		}
	}
}

func (e *CustomRuleEvaluator) evalPodPendingDuration(ctx context.Context, rule entity.AlertRule, clusterName string) {
	cs, err := e.clusters.ClientSet(clusterName)
	if err != nil {
		return
	}

	namespaces := resolveNamespaceScope(rule.Scope.Namespaces)
	ns := ""
	if len(namespaces) > 0 {
		ns = namespaces[0] // evaluate first namespace; iterate if needed
	}

	pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		FieldSelector: "status.phase=Pending",
	})
	if err != nil {
		return
	}

	threshold := parseDuration(rule.Condition.Window, 10*time.Minute)

	for _, pod := range pods.Items {
		if pod.CreationTimestamp.IsZero() {
			continue
		}
		pendingDuration := time.Since(pod.CreationTimestamp.Time)
		if pendingDuration > threshold {
			alertKey := fmt.Sprintf("custom:%s:%s/%s/%s", rule.Name, clusterName, pod.Namespace, pod.Name)
			if e.isInCooldown(alertKey) {
				continue
			}
			e.fireAlert(rule, clusterName, pod.Namespace, fmt.Sprintf("pod/%s", pod.Name),
				fmt.Sprintf("Pod stuck in Pending for %s (threshold: %s)",
					pendingDuration.Round(time.Second), threshold))
			e.markFired(alertKey)
		}
	}
}

func (e *CustomRuleEvaluator) evalNamespaceQuota(ctx context.Context, rule entity.AlertRule, clusterName string) {
	cs, err := e.clusters.ClientSet(clusterName)
	if err != nil {
		return
	}

	namespaces := resolveNamespaceScope(rule.Scope.Namespaces)
	if len(namespaces) == 0 {
		// List all namespaces
		nsList, listErr := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if listErr != nil {
			return
		}
		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
	}

	threshold := rule.Condition.Threshold
	if threshold <= 0 {
		threshold = 80
	}
	resourceType := rule.Condition.Resource
	if resourceType == "" {
		resourceType = "memory"
	}

	for _, ns := range namespaces {
		quotas, err := cs.CoreV1().ResourceQuotas(ns).List(ctx, metav1.ListOptions{})
		if err != nil || len(quotas.Items) == 0 {
			continue
		}

		for _, q := range quotas.Items {
			var resName corev1.ResourceName
			switch resourceType {
			case "cpu":
				resName = corev1.ResourceLimitsCPU
			default:
				resName = corev1.ResourceLimitsMemory
			}

			hard, hardOK := q.Status.Hard[resName]
			used, usedOK := q.Status.Used[resName]
			if !hardOK || !usedOK {
				// Try requests variant
				switch resourceType {
				case "cpu":
					resName = corev1.ResourceRequestsCPU
				default:
					resName = corev1.ResourceRequestsMemory
				}
				hard, hardOK = q.Status.Hard[resName]
				used, usedOK = q.Status.Used[resName]
			}
			if !hardOK || !usedOK || hard.IsZero() {
				continue
			}

			usedVal := used.Value()
			hardVal := hard.Value()
			if hardVal == 0 {
				continue
			}

			pct := int(usedVal * 100 / hardVal)
			if pct >= threshold {
				alertKey := fmt.Sprintf("custom:%s:%s/%s/%s", rule.Name, clusterName, ns, resourceType)
				if e.isInCooldown(alertKey) {
					continue
				}
				e.fireAlert(rule, clusterName, ns, fmt.Sprintf("quota/%s", q.Name),
					fmt.Sprintf("Namespace %s quota usage: %d%% (threshold: %d%%)\nUsed: %s / Hard: %s",
						resourceType, pct, threshold,
						formatQuantity(used), formatQuantity(hard)))
				e.markFired(alertKey)
			}
		}
	}
}

func (e *CustomRuleEvaluator) evalDeploymentUnavailable(ctx context.Context, rule entity.AlertRule, clusterName string) {
	cs, err := e.clusters.ClientSet(clusterName)
	if err != nil {
		return
	}

	namespaces := resolveNamespaceScope(rule.Scope.Namespaces)
	ns := ""
	if len(namespaces) > 0 {
		ns = namespaces[0]
	}

	deploys, err := cs.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, d := range deploys.Items {
		if d.Status.AvailableReplicas == 0 && d.Spec.Replicas != nil && *d.Spec.Replicas > 0 {
			alertKey := fmt.Sprintf("custom:%s:%s/%s/%s", rule.Name, clusterName, d.Namespace, d.Name)
			if e.isInCooldown(alertKey) {
				continue
			}
			e.fireAlert(rule, clusterName, d.Namespace, fmt.Sprintf("deployment/%s", d.Name),
				fmt.Sprintf("Deployment has 0 available replicas (desired: %d)",
					*d.Spec.Replicas))
			e.markFired(alertKey)
		}
	}
}

// ---------- Helpers ----------

func (e *CustomRuleEvaluator) fireAlert(rule entity.AlertRule, clusterName, namespace, resource, message string) {
	severity := mapSeverity(rule.Severity)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s *ALERT — %s*\n", severity.Emoji(), rule.Name))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if rule.Description != "" {
		sb.WriteString(fmt.Sprintf("_%s_\n\n", rule.Description))
	}

	sb.WriteString(fmt.Sprintf("Cluster:   %s\n", clusterName))
	sb.WriteString(fmt.Sprintf("Namespace: %s\n", namespace))
	sb.WriteString(fmt.Sprintf("Resource:  `%s`\n", resource))
	sb.WriteString(fmt.Sprintf("Severity:  %s %s\n", severity.Emoji(), rule.Severity))
	sb.WriteString(fmt.Sprintf("\n%s\n", message))
	sb.WriteString(fmt.Sprintf("\nTime: %s", time.Now().UTC().Format("2006-01-02 15:04:05 UTC")))

	alertText := sb.String()

	// Build mute button
	alertKey := fmt.Sprintf("custom:%s:%s/%s", rule.Name, clusterName, namespace)
	menu := &telebot.ReplyMarkup{}
	btnMute := menu.Data("🔇 Mute 1h", "watcher_mute", alertKey)
	menu.Inline(menu.Row(btnMute))

	// Send to configured chats (from rule.Notify or default)
	chats := rule.Notify.Chats
	if len(chats) == 0 {
		// Send to all configured chats + admins
		for _, chatID := range e.cfg.AllowedChats {
			if sendErr := e.notifier.SendAlert(chatID, alertText, menu); sendErr != nil {
				e.logger.Error("failed to send custom rule alert",
					zap.String("rule", rule.Name),
					zap.Int64("chat_id", chatID),
					zap.Error(sendErr),
				)
			}
		}
		for _, adminID := range e.cfg.AdminIDs {
			if sendErr := e.notifier.SendAlert(adminID, alertText, menu); sendErr != nil {
				e.logger.Error("failed to send custom rule alert to admin",
					zap.String("rule", rule.Name),
					zap.Int64("admin_id", adminID),
					zap.Error(sendErr),
				)
			}
		}
	} else {
		for _, chatID := range chats {
			if sendErr := e.notifier.SendAlert(chatID, alertText, menu); sendErr != nil {
				e.logger.Error("failed to send custom rule alert",
					zap.String("rule", rule.Name),
					zap.Int64("chat_id", chatID),
					zap.Error(sendErr),
				)
			}
		}
	}

	// Audit
	e.audit.Log(entity.AuditEntry{
		ID:        ulid.Make().String(),
		UserID:    0,
		Username:  "system:custom_rule",
		Action:    fmt.Sprintf("alert.custom.%s", rule.Name),
		Resource:  resource,
		Cluster:   clusterName,
		Namespace: namespace,
		Status:    entity.AuditStatusSuccess,
		Details: map[string]interface{}{
			"severity":    rule.Severity,
			"description": rule.Description,
			"message":     message,
		},
		OccurredAt: time.Now().UTC(),
	})

	e.logger.Info("custom rule alert fired",
		zap.String("rule", rule.Name),
		zap.String("cluster", clusterName),
		zap.String("namespace", namespace),
		zap.String("severity", rule.Severity),
	)
}

func (e *CustomRuleEvaluator) isInCooldown(key string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	lastFired, exists := e.alertCache[key]
	return exists && time.Since(lastFired) < customRuleCooldown
}

func (e *CustomRuleEvaluator) markFired(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.alertCache[key] = time.Now()
}

func mapSeverity(s string) AlertSeverity {
	switch strings.ToLower(s) {
	case "critical":
		return SeverityCritical
	case "warning":
		return SeverityWarning
	default:
		return AlertSeverity("info")
	}
}

func parseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

func formatQuantity(q resource.Quantity) string {
	return q.String()
}
