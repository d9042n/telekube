// Package watcher provides real-time Kubernetes monitoring with Telegram alerts.
package watcher

import (
	"context"
	"sync"
	"time"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// Notifier sends messages to Telegram chats.
type Notifier interface {
	SendAlert(chatID int64, text string, markup *telebot.ReplyMarkup) error
}

// Module implements the watcher module for real-time K8s monitoring.
type Module struct {
	clusters   cluster.Manager
	notifier   Notifier
	audit      audit.Logger
	logger     *zap.Logger
	cfg        config.TelegramConfig

	podWatcher         *PodWatcher
	nodeWatcher        *NodeWatcher
	argoCDWatchers     []*argoCDWatcher
	cronJobWatcher     *CronJobWatcher
	certWatcher        *CertWatcher
	pvcWatcher         *PVCWatcher
	customRuleEvaluator *CustomRuleEvaluator
	customRules        []entity.AlertRule

	mu       sync.RWMutex
	healthy  bool
	running  bool
	stopFunc context.CancelFunc
}

// NewModule creates a new watcher module.
func NewModule(
	clusters cluster.Manager,
	notifier Notifier,
	auditLogger audit.Logger,
	cfg config.TelegramConfig,
	logger *zap.Logger,
) *Module {
	return &Module{
		clusters: clusters,
		notifier: notifier,
		audit:    auditLogger,
		logger:   logger,
		cfg:      cfg,
		healthy:  true,
	}
}

func (m *Module) Name() string        { return "watcher" }
func (m *Module) Description() string { return "Real-time Kubernetes monitoring and alerts" }

func (m *Module) Register(bot *telebot.Bot, group *telebot.Group) {
	// Watchers don't register commands — they run in the background.
	// Alert buttons are handled by the kubernetes module or here.
	bot.Handle(&telebot.Btn{Unique: "watcher_mute"}, m.handleMute)
}

func (m *Module) Start(ctx context.Context) error {
	m.logger.Info("watcher module starting (watchers will start when leader elected)")
	return nil
}

// AddArgoCDWatcher registers an ArgoCD instance to be watched when started.
func (m *Module) AddArgoCDWatcher(cfg ArgoCDWatcherConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := newArgoCDWatcher(cfg, m.notifier, m.logger)
	m.argoCDWatchers = append(m.argoCDWatchers, w)
}

// SetCustomRules sets the custom alert rules to be evaluated by the watcher.
func (m *Module) SetCustomRules(rules []entity.AlertRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customRules = rules
}

// StartWatchers begins pod, node, and ArgoCD watcher goroutines.
// Should be called only by the leader replica.
func (m *Module) StartWatchers(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		m.logger.Warn("watchers already running")
		return
	}

	watchCtx, cancel := context.WithCancel(ctx)
	m.stopFunc = cancel

	// Create K8s watchers
	m.podWatcher = NewPodWatcher(m.clusters, m.notifier, m.audit, m.cfg, m.logger)
	m.nodeWatcher = NewNodeWatcher(m.clusters, m.notifier, m.audit, m.cfg, m.logger)
	m.cronJobWatcher = NewCronJobWatcher(m.clusters, m.notifier, m.audit, m.cfg, CronJobWatcherConfig{
		ExcludeNamespaces: []string{"kube-system"},
	}, m.logger)
	m.certWatcher = NewCertWatcher(m.clusters, m.notifier, m.audit, m.cfg, CertWatcherConfig{
		ExcludeNamespaces: []string{"kube-system"},
	}, m.logger)
	m.pvcWatcher = NewPVCWatcher(m.clusters, m.notifier, m.audit, m.cfg, PVCWatcherConfig{
		ExcludeNamespaces: []string{"kube-system"},
	}, m.logger)

	// Start watchers
	if err := m.podWatcher.Start(watchCtx); err != nil {
		m.logger.Error("failed to start pod watcher", zap.Error(err))
	}
	if err := m.nodeWatcher.Start(watchCtx); err != nil {
		m.logger.Error("failed to start node watcher", zap.Error(err))
	}
	if err := m.cronJobWatcher.Start(watchCtx); err != nil {
		m.logger.Error("failed to start cronjob watcher", zap.Error(err))
	}
	if err := m.certWatcher.Start(watchCtx); err != nil {
		m.logger.Error("failed to start cert watcher", zap.Error(err))
	}
	if err := m.pvcWatcher.Start(watchCtx); err != nil {
		m.logger.Error("failed to start pvc watcher", zap.Error(err))
	}

	// Start ArgoCD watchers
	for _, w := range m.argoCDWatchers {
		wCopy := w
		go wCopy.run(watchCtx)
	}

	// Start custom rule evaluator if configured
	if len(m.customRules) > 0 {
		m.customRuleEvaluator = NewCustomRuleEvaluator(
			m.customRules, m.clusters, m.notifier, m.audit, m.cfg, m.logger,
		)
		if err := m.customRuleEvaluator.Start(watchCtx); err != nil {
			m.logger.Error("failed to start custom rule evaluator", zap.Error(err))
		}
	}

	m.running = true
	m.logger.Info("watchers started",
		zap.Int("argocd_watchers", len(m.argoCDWatchers)),
		zap.Int("custom_rules", len(m.customRules)),
	)
}

// StopWatchers stops all running watchers.
func (m *Module) StopWatchers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	if m.stopFunc != nil {
		m.stopFunc()
	}

	m.running = false
	m.logger.Info("watchers stopped")
}

func (m *Module) Stop(ctx context.Context) error {
	m.StopWatchers()
	m.logger.Info("watcher module stopped")
	return nil
}

func (m *Module) Health() entity.HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.healthy {
		return entity.HealthStatusHealthy
	}
	return entity.HealthStatusUnhealthy
}

func (m *Module) Commands() []module.CommandInfo {
	// Watchers are background — no user-facing commands
	return nil
}

// handleMute handles the mute alert button.
func (m *Module) handleMute(c telebot.Context) error {
	// Muting is handled via alert deduplication cache extension
	_ = c.Respond(&telebot.CallbackResponse{Text: "🔇 Muted for 1 hour"})

	// Extend the cooldown for this alert to 1 hour
	if m.podWatcher != nil {
		data := c.Callback().Data
		m.podWatcher.muteAlert(data, 1*time.Hour)
	}
	if m.nodeWatcher != nil {
		data := c.Callback().Data
		m.nodeWatcher.muteAlert(data, 1*time.Hour)
	}

	return nil
}
