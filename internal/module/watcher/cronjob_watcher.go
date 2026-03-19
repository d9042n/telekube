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
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultCronCheckInterval = 5 * time.Minute
	suspendedThreshold       = 24 * time.Hour
)

// CronJobWatcherConfig holds configuration for the CronJob watcher.
type CronJobWatcherConfig struct {
	CheckInterval     time.Duration
	Namespaces        []string // empty = all
	ExcludeNamespaces []string
}

// CronJobWatcher monitors CronJob execution status and alerts on failures.
type CronJobWatcher struct {
	clusters   cluster.Manager
	notifier   Notifier
	audit      audit.Logger
	cfg        config.TelegramConfig
	watcherCfg CronJobWatcherConfig
	logger     *zap.Logger

	alertCache map[string]time.Time
	cooldown   time.Duration
	mu         sync.RWMutex
}

// NewCronJobWatcher creates a new CronJob watcher.
func NewCronJobWatcher(
	clusters cluster.Manager,
	notifier Notifier,
	auditLogger audit.Logger,
	cfg config.TelegramConfig,
	watcherCfg CronJobWatcherConfig,
	logger *zap.Logger,
) *CronJobWatcher {
	if watcherCfg.CheckInterval == 0 {
		watcherCfg.CheckInterval = defaultCronCheckInterval
	}
	return &CronJobWatcher{
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

// Start begins polling CronJob status across all clusters.
func (w *CronJobWatcher) Start(ctx context.Context) error {
	for _, c := range w.clusters.List() {
		clusterName := c.Name
		go wait.UntilWithContext(ctx, func(ctx context.Context) {
			w.checkCluster(ctx, clusterName)
		}, w.watcherCfg.CheckInterval)
	}
	go w.cleanupLoop(ctx)
	w.logger.Info("cronjob watcher started")
	return nil
}

// checkCluster checks all CronJobs in a cluster.
func (w *CronJobWatcher) checkCluster(ctx context.Context, clusterName string) {
	cs, err := w.clusters.ClientSet(clusterName)
	if err != nil {
		w.logger.Error("failed to get clientset for cronjob watcher",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return
	}

	namespaces := w.watcherCfg.Namespaces
	if len(namespaces) == 0 {
		nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			w.logger.Error("failed to list namespaces",
				zap.String("cluster", clusterName),
				zap.Error(err),
			)
			return
		}
		for _, ns := range nsList.Items {
			if !w.isExcluded(ns.Name) {
				namespaces = append(namespaces, ns.Name)
			}
		}
	}

	for _, ns := range namespaces {
		if w.isExcluded(ns) {
			continue
		}
		w.checkNamespace(ctx, cs, clusterName, ns)
	}
}

func (w *CronJobWatcher) isExcluded(ns string) bool {
	for _, excl := range w.watcherCfg.ExcludeNamespaces {
		if excl == ns {
			return true
		}
	}
	return false
}

// checkNamespace checks all CronJobs in a namespace.
func (w *CronJobWatcher) checkNamespace(ctx context.Context, cs kubernetes.Interface, clusterName, namespace string) {
	cronJobs, err := cs.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		w.logger.Error("failed to list cronjobs",
			zap.String("cluster", clusterName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return
	}

	for i := range cronJobs.Items {
		w.checkCronJob(ctx, cs, clusterName, &cronJobs.Items[i])
	}
}

// checkCronJob evaluates a single CronJob for alert conditions.
func (w *CronJobWatcher) checkCronJob(ctx context.Context, cs kubernetes.Interface, clusterName string, cj *batchv1.CronJob) {
	// Check if suspended for > 24h
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		if cj.Status.LastScheduleTime != nil && time.Since(cj.Status.LastScheduleTime.Time) > suspendedThreshold {
			w.alert(clusterName, cj, "Suspended", AlertSeverity("info"),
				fmt.Sprintf("CronJob suspended since %s",
					cj.Status.LastScheduleTime.Time.UTC().Format("2006-01-02 15:04 UTC")))
		}
		return
	}

	// Check for recent job failures
	w.checkRecentJobs(ctx, cs, clusterName, cj)
}

// checkRecentJobs checks the most recent jobs triggered by this CronJob.
func (w *CronJobWatcher) checkRecentJobs(ctx context.Context, cs kubernetes.Interface, clusterName string, cj *batchv1.CronJob) {
	jobs, err := cs.BatchV1().Jobs(cj.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for i := range jobs.Items {
		job := &jobs.Items[i]
		for _, ref := range job.OwnerReferences {
			if ref.Kind != "CronJob" || ref.Name != cj.Name {
				continue
			}
			// Only alert on recently failed jobs (within last 2 check intervals)
			if job.Status.Failed > 0 && job.Status.Succeeded == 0 {
				completionTime := time.Now()
				if job.Status.CompletionTime != nil {
					completionTime = job.Status.CompletionTime.Time
				}
				if time.Since(completionTime) > w.watcherCfg.CheckInterval*2 {
					break // Too old — already alerted or acknowledged
				}

				duration := "unknown"
				if job.Status.StartTime != nil {
					end := time.Now()
					if job.Status.CompletionTime != nil {
						end = job.Status.CompletionTime.Time
					}
					duration = end.Sub(job.Status.StartTime.Time).Round(time.Second).String()
				}

				w.alertJobFailed(clusterName, cj, job, duration)
			}
			break
		}
	}
}

// alertJobFailed sends a critical alert for a failed CronJob execution.
func (w *CronJobWatcher) alertJobFailed(clusterName string, cj *batchv1.CronJob, job *batchv1.Job, duration string) {
	alertKey := fmt.Sprintf("cronjob/%s/%s/%s/job/%s", clusterName, cj.Namespace, cj.Name, job.Name)

	w.mu.RLock()
	lastAlert, exists := w.alertCache[alertKey]
	w.mu.RUnlock()
	if exists && time.Since(lastAlert) < w.cooldown {
		return
	}
	w.mu.Lock()
	w.alertCache[alertKey] = time.Now()
	w.mu.Unlock()

	lastRun := "Unknown"
	if job.Status.StartTime != nil {
		lastRun = job.Status.StartTime.UTC().Format("2006-01-02 15:04:05 UTC")
	}

	var sb strings.Builder
	sb.WriteString("⏰ *CronJob Alert — Failed*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString(fmt.Sprintf("Cluster:    %s\n", clusterName))
	sb.WriteString(fmt.Sprintf("Namespace:  %s\n", cj.Namespace))
	sb.WriteString(fmt.Sprintf("CronJob:    `%s`\n", cj.Name))
	sb.WriteString(fmt.Sprintf("Schedule:   `%s`\n", cj.Spec.Schedule))
	sb.WriteString(fmt.Sprintf("Last Run:   %s\n", lastRun))
	sb.WriteString(fmt.Sprintf("Status:     🔴 Failed (attempts: %d)\n", job.Status.Failed))
	sb.WriteString(fmt.Sprintf("Duration:   %s\n", duration))

	menu := &telebot.ReplyMarkup{}
	cjData := fmt.Sprintf("%s|%s|%s", cj.Name, cj.Namespace, clusterName)
	jobData := fmt.Sprintf("%s|%s|%s", job.Name, cj.Namespace, clusterName)
	btnLogs := menu.Data("📋 Job Logs", "cj_job_logs", jobData)
	btnEvents := menu.Data("🔍 Events", "cj_events", cjData)
	btnTrigger := menu.Data("🔄 Trigger Now", "cj_trigger", cjData)
	btnMute := menu.Data("🔇 Mute", "watcher_mute", alertKey)
	menu.Inline(
		menu.Row(btnLogs, btnEvents),
		menu.Row(btnTrigger, btnMute),
	)

	w.sendCronJobAlert(sb.String(), menu)
	w.audit.Log(entity.AuditEntry{
		ID:        ulid.Make().String(),
		UserID:    0,
		Username:  "system:watcher",
		Action:    "alert.cronjob.failed",
		Resource:  fmt.Sprintf("cronjob/%s", cj.Name),
		Cluster:   clusterName,
		Namespace: cj.Namespace,
		Status:    entity.AuditStatusSuccess,
		Details: map[string]interface{}{
			"severity": "critical",
			"job":      job.Name,
			"attempts": job.Status.Failed,
		},
		OccurredAt: time.Now().UTC(),
	})

	w.logger.Info("cronjob failure alert sent",
		zap.String("cluster", clusterName),
		zap.String("namespace", cj.Namespace),
		zap.String("cronjob", cj.Name),
		zap.String("job", job.Name),
	)
}

// alert sends a generic CronJob alert (suspended, missed schedule, etc.).
func (w *CronJobWatcher) alert(clusterName string, cj *batchv1.CronJob, condition string, severity AlertSeverity, message string) {
	alertKey := fmt.Sprintf("cronjob/%s/%s/%s/%s", clusterName, cj.Namespace, cj.Name, condition)

	w.mu.RLock()
	lastAlert, exists := w.alertCache[alertKey]
	w.mu.RUnlock()
	if exists && time.Since(lastAlert) < w.cooldown {
		return
	}
	w.mu.Lock()
	w.alertCache[alertKey] = time.Now()
	w.mu.Unlock()

	lastRun := "Never"
	if cj.Status.LastScheduleTime != nil {
		lastRun = cj.Status.LastScheduleTime.UTC().Format("2006-01-02 15:04:05 UTC")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⏰ *CronJob Alert — %s*\n", condition))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString(fmt.Sprintf("Cluster:    %s\n", clusterName))
	sb.WriteString(fmt.Sprintf("Namespace:  %s\n", cj.Namespace))
	sb.WriteString(fmt.Sprintf("CronJob:    `%s`\n", cj.Name))
	sb.WriteString(fmt.Sprintf("Schedule:   `%s`\n", cj.Spec.Schedule))
	sb.WriteString(fmt.Sprintf("Last Run:   %s\n", lastRun))
	sb.WriteString(fmt.Sprintf("Status:     %s %s\n", severity.Emoji(), condition))
	sb.WriteString(fmt.Sprintf("Detail:     %s\n", message))

	menu := &telebot.ReplyMarkup{}
	cjData := fmt.Sprintf("%s|%s|%s", cj.Name, cj.Namespace, clusterName)
	btnEvents := menu.Data("🔍 Events", "cj_events", cjData)
	btnMute := menu.Data("🔇 Mute", "watcher_mute", alertKey)
	menu.Inline(menu.Row(btnEvents, btnMute))

	w.sendCronJobAlert(sb.String(), menu)
}

func (w *CronJobWatcher) sendCronJobAlert(text string, markup *telebot.ReplyMarkup) {
	for _, chatID := range w.cfg.AllowedChats {
		if err := w.notifier.SendAlert(chatID, text, markup); err != nil {
			w.logger.Error("failed to send cronjob alert to chat",
				zap.Int64("chat_id", chatID),
				zap.Error(err),
			)
		}
	}
	for _, adminID := range w.cfg.AdminIDs {
		if err := w.notifier.SendAlert(adminID, text, markup); err != nil {
			w.logger.Error("failed to send cronjob alert to admin",
				zap.Int64("admin_id", adminID),
				zap.Error(err),
			)
		}
	}
}

func (w *CronJobWatcher) cleanupLoop(ctx context.Context) {
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
