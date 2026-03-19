// Package briefing provides scheduled cluster health reports.
package briefing

import (
	"context"
	"sync"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/module/watcher"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// Module implements the briefing feature for scheduled reports.
type Module struct {
	scheduler *Scheduler
	reporter  *Reporter
	notifier  watcher.Notifier
	logger    *zap.Logger
	cfg       Config

	mu       sync.RWMutex
	healthy  bool
	running  bool
	stopFunc context.CancelFunc
}

// Config holds briefing-specific configuration.
type Config struct {
	Enabled  bool
	Schedule string // Cron expression
	Timezone string
	Chats    []int64 // Empty = all allowed_chats
}

// NewModule creates a new briefing module.
func NewModule(
	clusters cluster.Manager,
	notifier watcher.Notifier,
	auditLogger audit.Logger,
	telegramCfg config.TelegramConfig,
	briefingCfg Config,
	logger *zap.Logger,
) *Module {
	reporter := NewReporter(clusters, auditLogger, logger)

	return &Module{
		reporter: reporter,
		notifier: notifier,
		logger:   logger,
		cfg:      briefingCfg,
		healthy:  true,
	}
}

func (m *Module) Name() string        { return "briefing" }
func (m *Module) Description() string { return "Scheduled cluster health reports" }

func (m *Module) Register(bot *telebot.Bot, group *telebot.Group) {
	// Briefing is triggered by scheduler, no user commands
}

func (m *Module) Start(ctx context.Context) error {
	m.logger.Info("briefing module starting (scheduler will start when leader elected)")
	return nil
}

// StartScheduler starts the scheduled briefing. Called only by leader.
func (m *Module) StartScheduler(ctx context.Context, chats []int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return
	}

	schedCtx, cancel := context.WithCancel(ctx)
	m.stopFunc = cancel

	targetChats := chats
	if len(m.cfg.Chats) > 0 {
		targetChats = m.cfg.Chats
	}

	m.scheduler = NewScheduler(m.reporter, m.notifier, m.cfg, targetChats, m.logger)
	m.scheduler.Start(schedCtx)

	m.running = true
	m.logger.Info("briefing scheduler started",
		zap.String("schedule", m.cfg.Schedule),
		zap.String("timezone", m.cfg.Timezone),
	)
}

// StopScheduler stops the scheduler.
func (m *Module) StopScheduler() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	if m.scheduler != nil {
		m.scheduler.Stop()
	}
	if m.stopFunc != nil {
		m.stopFunc()
	}

	m.running = false
	m.logger.Info("briefing scheduler stopped")
}

func (m *Module) Stop(ctx context.Context) error {
	m.StopScheduler()
	m.logger.Info("briefing module stopped")
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
	return nil // Briefing is automatic, no commands
}
