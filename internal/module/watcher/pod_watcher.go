package watcher

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/bot/keyboard"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

const (
	defaultCooldown   = 5 * time.Minute
	cacheCleanupEvery = 10 * time.Minute
	pendingThreshold  = 5 * time.Minute
	restartThreshold  = 5
	resyncPeriod      = 30 * time.Second
)

// AlertSeverity represents the severity of an alert.
type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityWarning  AlertSeverity = "warning"
)

func (s AlertSeverity) Emoji() string {
	switch s {
	case SeverityCritical:
		return "🔴"
	case SeverityWarning:
		return "🟡"
	default:
		return "⚪"
	}
}

// PodWatcher watches for pod issues across clusters.
type PodWatcher struct {
	clusters   cluster.Manager
	notifier   Notifier
	audit      audit.Logger
	cfg        config.TelegramConfig
	logger     *zap.Logger

	alertCache map[string]time.Time // key: cluster/ns/pod/condition
	cooldown   time.Duration
	mu         sync.RWMutex
}

// NewPodWatcher creates a new pod watcher.
func NewPodWatcher(
	clusters cluster.Manager,
	notifier Notifier,
	auditLogger audit.Logger,
	cfg config.TelegramConfig,
	logger *zap.Logger,
) *PodWatcher {
	return &PodWatcher{
		clusters:   clusters,
		notifier:   notifier,
		audit:      auditLogger,
		cfg:        cfg,
		logger:     logger,
		alertCache: make(map[string]time.Time),
		cooldown:   defaultCooldown,
	}
}

// Start begins watching pods on all clusters.
func (w *PodWatcher) Start(ctx context.Context) error {
	for _, c := range w.clusters.List() {
		go w.watchCluster(ctx, c.Name)
	}

	// Start cache cleanup goroutine
	go w.cleanupLoop(ctx)

	w.logger.Info("pod watcher started")
	return nil
}

// watchCluster sets up a pod informer for a single cluster.
func (w *PodWatcher) watchCluster(ctx context.Context, clusterName string) {
	clientset, err := w.clusters.ClientSet(clusterName)
	if err != nil {
		w.logger.Error("failed to get clientset for pod watcher",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return
	}

	factory := informers.NewSharedInformerFactory(clientset, resyncPeriod)
	podInformer := factory.Core().V1().Pods().Informer()

	_, err = podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, new interface{}) {
			oldPod, ok1 := old.(*corev1.Pod)
			newPod, ok2 := new.(*corev1.Pod)
			if ok1 && ok2 {
				w.checkPod(clusterName, oldPod, newPod)
			}
		},
	})
	if err != nil {
		w.logger.Error("failed to add event handler",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return
	}

	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())

	w.logger.Info("pod informer started", zap.String("cluster", clusterName))
	<-ctx.Done()
	w.logger.Info("pod informer stopped", zap.String("cluster", clusterName))
}

// checkPod checks for conditions that should trigger alerts.
func (w *PodWatcher) checkPod(cluster string, oldPod, newPod *corev1.Pod) {
	// Check container statuses
	for _, cs := range newPod.Status.ContainerStatuses {
		// OOMKilled
		if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
			w.alert(cluster, newPod, "OOMKilled", cs.Name, SeverityCritical,
				"Container exceeded memory limit")
		}

		// CrashLoopBackOff
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
			w.alert(cluster, newPod, "CrashLoopBackOff", cs.Name, SeverityCritical,
				fmt.Sprintf("Container crashing repeatedly (restarts: %d)", cs.RestartCount))
		}

		// ImagePullBackOff
		if cs.State.Waiting != nil &&
			(cs.State.Waiting.Reason == "ImagePullBackOff" || cs.State.Waiting.Reason == "ErrImagePull") {
			w.alert(cluster, newPod, "ImagePullBackOff", cs.Name, SeverityWarning,
				fmt.Sprintf("Cannot pull image: %s", cs.Image))
		}

		// High restart count
		if cs.RestartCount >= restartThreshold {
			// Only alert on restart count change
			for _, oldCS := range oldPod.Status.ContainerStatuses {
				if oldCS.Name == cs.Name && cs.RestartCount > oldCS.RestartCount {
					w.alert(cluster, newPod, "HighRestarts", cs.Name, SeverityWarning,
						fmt.Sprintf("Container restart count: %d", cs.RestartCount))
				}
			}
		}
	}

	// Pod Pending > 5 minutes
	if newPod.Status.Phase == corev1.PodPending {
		if !newPod.CreationTimestamp.IsZero() && time.Since(newPod.CreationTimestamp.Time) > pendingThreshold {
			w.alert(cluster, newPod, "PendingTooLong", "", SeverityWarning,
				fmt.Sprintf("Pod stuck in Pending for %s", time.Since(newPod.CreationTimestamp.Time).Round(time.Second)))
		}
	}

	// Pod Evicted
	if newPod.Status.Phase == corev1.PodFailed && newPod.Status.Reason == "Evicted" {
		// Check it wasn't already failed
		if oldPod.Status.Phase != corev1.PodFailed {
			w.alert(cluster, newPod, "Evicted", "", SeverityCritical,
				fmt.Sprintf("Pod evicted: %s", newPod.Status.Message))
		}
	}
}

// alert sends an alert to configured chats if not in cooldown.
func (w *PodWatcher) alert(cluster string, pod *corev1.Pod, condition, container string, severity AlertSeverity, message string) {
	alertKey := fmt.Sprintf("%s/%s/%s/%s", cluster, pod.Namespace, pod.Name, condition)

	w.mu.RLock()
	lastAlert, exists := w.alertCache[alertKey]
	w.mu.RUnlock()

	if exists && time.Since(lastAlert) < w.cooldown {
		return // Deduplicated
	}

	w.mu.Lock()
	w.alertCache[alertKey] = time.Now()
	w.mu.Unlock()

	// Build alert message
	var sb strings.Builder
	sb.WriteString("🚨 *ALERT — Pod Issue Detected*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString(fmt.Sprintf("Cluster:   %s\n", cluster))
	sb.WriteString(fmt.Sprintf("Namespace: %s\n", pod.Namespace))
	sb.WriteString(fmt.Sprintf("Pod:       `%s`\n", pod.Name))
	sb.WriteString(fmt.Sprintf("Condition: %s %s\n", severity.Emoji(), condition))
	if container != "" {
		sb.WriteString(fmt.Sprintf("Container: %s\n", container))
	}
	sb.WriteString(fmt.Sprintf("Message:   %s\n", message))
	sb.WriteString(fmt.Sprintf("Time:      %s\n", time.Now().UTC().Format("2006-01-02 15:04:05 UTC")))

	// Action buttons
	menu := &telebot.ReplyMarkup{}
	podData := keyboard.GlobalStore().Store(fmt.Sprintf("%s|%s|%s", pod.Name, pod.Namespace, cluster))
	btnLogs := menu.Data("📋 Full Logs", "k8s_logs", podData)
	btnEvents := menu.Data("🔍 Events", "k8s_events", podData)
	btnRestart := menu.Data("🔄 Restart", "k8s_restart", podData)
	btnMute := menu.Data("🔇 Mute 1h", "watcher_mute", alertKey)

	menu.Inline(
		menu.Row(btnLogs, btnEvents),
		menu.Row(btnRestart, btnMute),
	)

	alertText := sb.String()

	// Send to all configured chats
	w.sendToChats(alertText, menu)

	// Audit the alert
	w.audit.Log(entity.AuditEntry{
		ID:        ulid.Make().String(),
		UserID:    0, // System
		Username:  "system:watcher",
		Action:    fmt.Sprintf("alert.%s", strings.ToLower(condition)),
		Resource:  fmt.Sprintf("pod/%s", pod.Name),
		Cluster:   cluster,
		Namespace: pod.Namespace,
		Status:    entity.AuditStatusSuccess,
		Details: map[string]interface{}{
			"severity":  string(severity),
			"condition": condition,
			"container": container,
			"message":   message,
		},
		OccurredAt: time.Now().UTC(),
	})

	w.logger.Info("pod alert sent",
		zap.String("cluster", cluster),
		zap.String("pod", pod.Name),
		zap.String("namespace", pod.Namespace),
		zap.String("condition", condition),
		zap.String("severity", string(severity)),
	)
}

// muteAlert extends the cooldown for a specific alert key.
func (w *PodWatcher) muteAlert(alertKey string, duration time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.alertCache[alertKey] = time.Now().Add(duration - w.cooldown)
}

// sendToChats sends a message to all configured alert chats.
func (w *PodWatcher) sendToChats(text string, markup *telebot.ReplyMarkup) {
	for _, chatID := range w.cfg.AllowedChats {
		if err := w.notifier.SendAlert(chatID, text, markup); err != nil {
			w.logger.Error("failed to send alert to chat",
				zap.Int64("chat_id", chatID),
				zap.Error(err),
			)
		}
	}
	// Also send to admin users
	for _, adminID := range w.cfg.AdminIDs {
		if err := w.notifier.SendAlert(adminID, text, markup); err != nil {
			w.logger.Error("failed to send alert to admin",
				zap.Int64("admin_id", adminID),
				zap.Error(err),
			)
		}
	}
}

// cleanupLoop periodically cleans up expired alert cache entries.
func (w *PodWatcher) cleanupLoop(ctx context.Context) {
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
