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
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// NodeWatcher watches for node condition changes across clusters.
type NodeWatcher struct {
	clusters   cluster.Manager
	notifier   Notifier
	audit      audit.Logger
	cfg        config.TelegramConfig
	logger     *zap.Logger

	alertCache map[string]time.Time
	cooldown   time.Duration
	mu         sync.RWMutex
}

// NewNodeWatcher creates a new node watcher.
func NewNodeWatcher(
	clusters cluster.Manager,
	notifier Notifier,
	auditLogger audit.Logger,
	cfg config.TelegramConfig,
	logger *zap.Logger,
) *NodeWatcher {
	return &NodeWatcher{
		clusters:   clusters,
		notifier:   notifier,
		audit:      auditLogger,
		cfg:        cfg,
		logger:     logger,
		alertCache: make(map[string]time.Time),
		cooldown:   defaultCooldown,
	}
}

// Start begins watching nodes on all clusters.
func (w *NodeWatcher) Start(ctx context.Context) error {
	for _, c := range w.clusters.List() {
		go w.watchCluster(ctx, c.Name)
	}

	go w.cleanupLoop(ctx)

	w.logger.Info("node watcher started")
	return nil
}

// watchCluster sets up a node informer for a single cluster.
func (w *NodeWatcher) watchCluster(ctx context.Context, clusterName string) {
	clientset, err := w.clusters.ClientSet(clusterName)
	if err != nil {
		w.logger.Error("failed to get clientset for node watcher",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return
	}

	factory := informers.NewSharedInformerFactory(clientset, resyncPeriod)
	nodeInformer := factory.Core().V1().Nodes().Informer()

	_, err = nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, new interface{}) {
			oldNode, ok1 := old.(*corev1.Node)
			newNode, ok2 := new.(*corev1.Node)
			if ok1 && ok2 {
				w.checkNode(clusterName, oldNode, newNode)
			}
		},
	})
	if err != nil {
		w.logger.Error("failed to add node event handler",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return
	}

	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())

	w.logger.Info("node informer started", zap.String("cluster", clusterName))
	<-ctx.Done()
	w.logger.Info("node informer stopped", zap.String("cluster", clusterName))
}

// checkNode checks for condition changes that should trigger alerts.
func (w *NodeWatcher) checkNode(clusterName string, oldNode, newNode *corev1.Node) {
	// Build condition maps
	oldConditions := make(map[corev1.NodeConditionType]corev1.ConditionStatus)
	for _, c := range oldNode.Status.Conditions {
		oldConditions[c.Type] = c.Status
	}

	for _, cond := range newNode.Status.Conditions {
		oldStatus := oldConditions[cond.Type]

		switch cond.Type {
		case corev1.NodeReady:
			// Ready -> False (NotReady)
			if oldStatus == corev1.ConditionTrue && cond.Status == corev1.ConditionFalse {
				w.alert(clusterName, newNode, "NotReady", SeverityCritical,
					cond.Reason, cond.Message, cond.LastTransitionTime.Time)
			}
			// Ready -> Unknown (Unreachable)
			if oldStatus == corev1.ConditionTrue && cond.Status == corev1.ConditionUnknown {
				w.alert(clusterName, newNode, "Unreachable", SeverityCritical,
					cond.Reason, cond.Message, cond.LastTransitionTime.Time)
			}

		case corev1.NodeDiskPressure:
			if oldStatus != corev1.ConditionTrue && cond.Status == corev1.ConditionTrue {
				w.alert(clusterName, newNode, "DiskPressure", SeverityWarning,
					cond.Reason, cond.Message, cond.LastTransitionTime.Time)
			}

		case corev1.NodeMemoryPressure:
			if oldStatus != corev1.ConditionTrue && cond.Status == corev1.ConditionTrue {
				w.alert(clusterName, newNode, "MemoryPressure", SeverityWarning,
					cond.Reason, cond.Message, cond.LastTransitionTime.Time)
			}

		case corev1.NodePIDPressure:
			if oldStatus != corev1.ConditionTrue && cond.Status == corev1.ConditionTrue {
				w.alert(clusterName, newNode, "PIDPressure", SeverityWarning,
					cond.Reason, cond.Message, cond.LastTransitionTime.Time)
			}
		}
	}
}

// alert sends a node alert to configured chats if not in cooldown.
func (w *NodeWatcher) alert(clusterName string, node *corev1.Node, condition string, severity AlertSeverity, reason, message string, since time.Time) {
	alertKey := fmt.Sprintf("%s/%s/%s", clusterName, node.Name, condition)

	w.mu.RLock()
	lastAlert, exists := w.alertCache[alertKey]
	w.mu.RUnlock()

	if exists && time.Since(lastAlert) < w.cooldown {
		return // Deduplicated
	}

	w.mu.Lock()
	w.alertCache[alertKey] = time.Now()
	w.mu.Unlock()

	// Count affected pods
	affectedPods := w.countAffectedPods(clusterName, node.Name)

	// Build alert message
	var sb strings.Builder
	sb.WriteString("🚨 *NODE ALERT* — ")
	switch severity {
	case SeverityCritical:
		sb.WriteString("Critical\n")
	case SeverityWarning:
		sb.WriteString("Warning\n")
	}
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString(fmt.Sprintf("Cluster:   %s\n", clusterName))
	sb.WriteString(fmt.Sprintf("Node:      `%s`\n", node.Name))
	sb.WriteString(fmt.Sprintf("Condition: %s %s\n", severity.Emoji(), condition))
	if !since.IsZero() {
		sb.WriteString(fmt.Sprintf("Since:     %s\n", since.UTC().Format("2006-01-02 15:04:05 UTC")))
	}
	if reason != "" {
		sb.WriteString(fmt.Sprintf("Reason:    %s\n", reason))
	}
	if message != "" {
		sb.WriteString(fmt.Sprintf("Message:   %s\n", message))
	}
	sb.WriteString(fmt.Sprintf("\nAffected Pods: %d running on this node\n", affectedPods))

	// Action buttons
	menu := &telebot.ReplyMarkup{}
	nodeData := fmt.Sprintf("%s|%s", node.Name, clusterName)
	btnDetail := menu.Data("📊 Node Details", "k8s_node_detail", nodeData)
	btnCordon := menu.Data("🔒 Cordon", "k8s_node_cordon", nodeData)
	btnDrain := menu.Data("📤 Drain", "k8s_node_drain", nodeData)
	btnMute := menu.Data("🔇 Mute 1h", "watcher_mute", alertKey)

	menu.Inline(
		menu.Row(btnDetail, btnCordon),
		menu.Row(btnDrain, btnMute),
	)

	alertText := sb.String()
	w.sendToChats(alertText, menu)

	// Audit the alert
	w.audit.Log(entity.AuditEntry{
		ID:        ulid.Make().String(),
		UserID:    0,
		Username:  "system:watcher",
		Action:    fmt.Sprintf("alert.node.%s", strings.ToLower(condition)),
		Resource:  fmt.Sprintf("node/%s", node.Name),
		Cluster:   clusterName,
		Status:    entity.AuditStatusSuccess,
		Details: map[string]interface{}{
			"severity":      string(severity),
			"condition":     condition,
			"reason":        reason,
			"affected_pods": affectedPods,
		},
		OccurredAt: time.Now().UTC(),
	})

	w.logger.Info("node alert sent",
		zap.String("cluster", clusterName),
		zap.String("node", node.Name),
		zap.String("condition", condition),
		zap.String("severity", string(severity)),
	)
}

// countAffectedPods counts pods running on a specific node.
func (w *NodeWatcher) countAffectedPods(clusterName, nodeName string) int {
	clientset, err := w.clusters.ClientSet(clusterName)
	if err != nil {
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", nodeName).String(),
	})
	if err != nil {
		return 0
	}
	return len(pods.Items)
}

// muteAlert extends the cooldown for a specific alert key.
func (w *NodeWatcher) muteAlert(alertKey string, duration time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.alertCache[alertKey] = time.Now().Add(duration - w.cooldown)
}

// sendToChats sends a message to all configured alert chats.
func (w *NodeWatcher) sendToChats(text string, markup *telebot.ReplyMarkup) {
	for _, chatID := range w.cfg.AllowedChats {
		if err := w.notifier.SendAlert(chatID, text, markup); err != nil {
			w.logger.Error("failed to send node alert to chat",
				zap.Int64("chat_id", chatID),
				zap.Error(err),
			)
		}
	}
	for _, adminID := range w.cfg.AdminIDs {
		if err := w.notifier.SendAlert(adminID, text, markup); err != nil {
			w.logger.Error("failed to send node alert to admin",
				zap.Int64("admin_id", adminID),
				zap.Error(err),
			)
		}
	}
}

// cleanupLoop periodically cleans up expired alert cache entries.
func (w *NodeWatcher) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(cacheCleanupEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.mu.Lock()
			for key, t := range w.alertCache {
				if time.Since(t) > w.cooldown {
					delete(w.alertCache, key)
				}
			}
			w.mu.Unlock()
		}
	}
}
