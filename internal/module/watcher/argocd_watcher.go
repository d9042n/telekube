package watcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	pkgargocd "github.com/d9042n/telekube/pkg/argocd"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// ArgoCDWatcherConfig holds configuration for the ArgoCD watcher.
type ArgoCDWatcherConfig struct {
	// InstanceName is the name of the ArgoCD instance being watched.
	InstanceName string
	// Client is the ArgoCD API client.
	Client pkgargocd.Client
	// PollInterval controls how often the watcher polls for changes.
	PollInterval time.Duration
	// Clusters maps ArgoCD instance to K8s cluster names for context links.
	Clusters []string
	// NotifyOnHealthChange alerts when an app's health status changes.
	NotifyOnHealthChange bool
	// NotifyOnSyncChange alerts when an app's sync status changes.
	NotifyOnSyncChange bool
}

// argoCDWatcher polls ArgoCD for application status changes and sends Telegram alerts.
type argoCDWatcher struct {
	cfg      ArgoCDWatcherConfig
	notifier Notifier
	logger   *zap.Logger

	// last known state: appName → Application
	lastState map[string]pkgargocd.Application
}

// newArgoCDWatcher creates a new ArgoCD watcher.
func newArgoCDWatcher(cfg ArgoCDWatcherConfig, notifier Notifier, logger *zap.Logger) *argoCDWatcher {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}
	return &argoCDWatcher{
		cfg:       cfg,
		notifier:  notifier,
		logger:    logger,
		lastState: make(map[string]pkgargocd.Application),
	}
}

// run starts the polling loop and blocks until ctx is cancelled.
func (w *argoCDWatcher) run(ctx context.Context) {
	w.logger.Info("argocd watcher started",
		zap.String("instance", w.cfg.InstanceName),
		zap.Duration("poll_interval", w.cfg.PollInterval),
	)

	// Seed initial state without alerting
	w.seedInitialState(ctx)

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("argocd watcher stopped",
				zap.String("instance", w.cfg.InstanceName),
			)
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

// seedInitialState loads the current state without alerting.
func (w *argoCDWatcher) seedInitialState(ctx context.Context) {
	pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	apps, err := w.cfg.Client.ListApplications(pollCtx, pkgargocd.ListOpts{})
	if err != nil {
		w.logger.Warn("argocd watcher: failed to fetch initial state",
			zap.String("instance", w.cfg.InstanceName),
			zap.Error(err),
		)
		return
	}
	for _, app := range apps {
		w.lastState[app.Name] = app
	}
	w.logger.Debug("argocd watcher: seeded initial state",
		zap.String("instance", w.cfg.InstanceName),
		zap.Int("app_count", len(apps)),
	)
}

// poll fetches current application states and fires alerts on changes.
func (w *argoCDWatcher) poll(ctx context.Context) {
	pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	apps, err := w.cfg.Client.ListApplications(pollCtx, pkgargocd.ListOpts{})
	if err != nil {
		w.logger.Warn("argocd watcher: poll failed",
			zap.String("instance", w.cfg.InstanceName),
			zap.Error(err),
		)
		return
	}

	currentNames := make(map[string]struct{}, len(apps))
	for _, app := range apps {
		currentNames[app.Name] = struct{}{}
		prev, exists := w.lastState[app.Name]

		if !exists {
			// New app detected
			w.notifyNewApp(app)
			w.lastState[app.Name] = app
			continue
		}

		// Check for status changes
		healthChanged := prev.HealthStatus != app.HealthStatus
		syncChanged := prev.SyncStatus != app.SyncStatus

		if (healthChanged && w.cfg.NotifyOnHealthChange) || (syncChanged && w.cfg.NotifyOnSyncChange) {
			w.notifyStatusChange(prev, app)
		}
		w.lastState[app.Name] = app
	}

	// Detect deleted apps
	for name := range w.lastState {
		if _, stillExists := currentNames[name]; !stillExists {
			w.notifyDeletedApp(w.lastState[name])
			delete(w.lastState, name)
		}
	}
}

// notifyNewApp sends an alert when a new ArgoCD application is discovered.
func (w *argoCDWatcher) notifyNewApp(app pkgargocd.Application) {
	msg := fmt.Sprintf("🆕 *New ArgoCD Application*\n\n"+
		"Instance: %s\n"+
		"App:      `%s`\n"+
		"Project:  %s\n"+
		"Sync:     %s | Health: %s",
		w.cfg.InstanceName, app.Name, app.Project,
		app.SyncStatus, app.HealthStatus)
	w.send(msg)
}

// notifyStatusChange sends an alert on sync or health status transitions.
func (w *argoCDWatcher) notifyStatusChange(prev, curr pkgargocd.Application) {
	var sb strings.Builder
	emoji := statusChangeEmoji(curr.SyncStatus, curr.HealthStatus)
	sb.WriteString(fmt.Sprintf("%s *ArgoCD Status Change*\n\n", emoji))
	sb.WriteString(fmt.Sprintf("Instance: %s\n", w.cfg.InstanceName))
	sb.WriteString(fmt.Sprintf("App:      `%s`\n", curr.Name))
	sb.WriteString(fmt.Sprintf("Project:  %s\n", curr.Project))

	if prev.SyncStatus != curr.SyncStatus {
		sb.WriteString(fmt.Sprintf("Sync:     %s → *%s*\n", prev.SyncStatus, curr.SyncStatus))
	}
	if prev.HealthStatus != curr.HealthStatus {
		sb.WriteString(fmt.Sprintf("Health:   %s → *%s*\n", prev.HealthStatus, curr.HealthStatus))
	}
	if curr.CurrentRev != "" {
		sb.WriteString(fmt.Sprintf("Revision: `%s`\n", shortRevWatcher(curr.CurrentRev)))
	}
	sb.WriteString(fmt.Sprintf("Time:     %s", time.Now().UTC().Format("2006-01-02 15:04:05 UTC")))

	w.send(sb.String())
}

// notifyDeletedApp sends an alert when an ArgoCD application is removed.
func (w *argoCDWatcher) notifyDeletedApp(app pkgargocd.Application) {
	msg := fmt.Sprintf("🗑️ *ArgoCD Application Removed*\n\n"+
		"Instance: %s\n"+
		"App:      `%s`\n"+
		"Project:  %s",
		w.cfg.InstanceName, app.Name, app.Project)
	w.send(msg)
}

func (w *argoCDWatcher) send(text string) {
	if w.notifier == nil {
		return
	}
	// Use SendAlert with a nil markup since this is a broadcast-style notification
	_ = w.notifier.SendAlert(0, text, (*telebot.ReplyMarkup)(nil))
}

// statusChangeEmoji selects an emoji for a sync/health combination.
func statusChangeEmoji(syncStatus, healthStatus string) string {
	if syncStatus == "OutOfSync" {
		return "🟡"
	}
	if healthStatus == "Degraded" || healthStatus == "Missing" {
		return "🔴"
	}
	if syncStatus == "Synced" && healthStatus == "Healthy" {
		return "✅"
	}
	return "ℹ️"
}

// shortRevWatcher returns the first 7 chars of a git revision.
func shortRevWatcher(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}
