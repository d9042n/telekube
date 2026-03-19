// Package app manages the application lifecycle.
package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/bot"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/leader"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/module/briefing"
	"github.com/d9042n/telekube/internal/module/watcher"
	"github.com/d9042n/telekube/internal/storage"
	pkgredis "github.com/d9042n/telekube/pkg/redis"
	"github.com/d9042n/telekube/pkg/health"
	"go.uber.org/zap"
)

// App manages the application lifecycle.
type App struct {
	bot       *bot.Bot
	registry  *module.Registry
	health    *health.Server
	audit     audit.Logger
	storage   storage.Storage
	logger    *zap.Logger

	// Phase 2 optional components
	redis     *pkgredis.Client
	watcher   *watcher.Module
	briefing  *briefing.Module
	leCfg     *config.LeaderElectionConfig
	clusterMgr cluster.Manager
}

// Option configures the App.
type Option func(*App)

// WithRedis sets the Redis client.
func WithRedis(client *pkgredis.Client) Option {
	return func(a *App) {
		a.redis = client
	}
}

// WithWatcher sets the watcher module.
func WithWatcher(w *watcher.Module) Option {
	return func(a *App) {
		a.watcher = w
	}
}

// WithBriefing sets the briefing module.
func WithBriefing(b *briefing.Module) Option {
	return func(a *App) {
		a.briefing = b
	}
}

// WithLeaderElection configures leader election.
func WithLeaderElection(cfg config.LeaderElectionConfig, mgr cluster.Manager) Option {
	return func(a *App) {
		a.leCfg = &cfg
		a.clusterMgr = mgr
	}
}

// NewApp creates a new App with the given components.
func NewApp(
	teleBot *bot.Bot,
	registry *module.Registry,
	healthSrv *health.Server,
	auditLogger audit.Logger,
	store storage.Storage,
	logger *zap.Logger,
	opts ...Option,
) *App {
	a := &App{
		bot:      teleBot,
		registry: registry,
		health:   healthSrv,
		audit:    auditLogger,
		storage:  store,
		logger:   logger,
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

// Run starts all components and blocks until shutdown.
func (a *App) Run(ctx context.Context) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start modules
	if err := a.registry.StartAll(ctx); err != nil {
		a.logger.Error("failed to start modules", zap.Error(err))
	}

	// Start health server
	go func() {
		if err := a.health.Start(); err != nil {
			a.logger.Error("health server error", zap.Error(err))
		}
	}()

	// Start leader election or run watchers directly
	if a.leCfg != nil && a.leCfg.Enabled {
		a.startWithLeaderElection(ctx)
	} else {
		// No leader election: run watchers/briefing directly
		a.startBackgroundTasks(ctx)
	}

	// Start bot (blocking)
	go a.bot.Start()
	a.logger.Info("telekube is running")

	// Wait for shutdown
	<-ctx.Done()
	a.logger.Info("shutting down...")
	a.shutdown()
}

// startBackgroundTasks starts watchers and briefing without leader election.
func (a *App) startBackgroundTasks(ctx context.Context) {
	if a.watcher != nil {
		a.watcher.StartWatchers(ctx)
	}
	if a.briefing != nil {
		// Determine target chats
		// This would normally come from config
		a.briefing.StartScheduler(ctx, nil)
	}
}

// startWithLeaderElection sets up leader election callbacks.
func (a *App) startWithLeaderElection(ctx context.Context) {
	// Use the default cluster for leader election
	defaultCluster, err := a.clusterMgr.GetDefault()
	if err != nil {
		a.logger.Error("no default cluster for leader election, falling back to direct mode",
			zap.Error(err),
		)
		a.startBackgroundTasks(ctx)
		return
	}

	clientset, err := a.clusterMgr.ClientSet(defaultCluster.Name)
	if err != nil {
		a.logger.Error("failed to get clientset for leader election",
			zap.Error(err),
		)
		a.startBackgroundTasks(ctx)
		return
	}

	namespace := a.leCfg.Namespace
	if namespace == "" {
		namespace = "default"
	}

	leCfg := leader.DefaultConfig(namespace)

	callbacks := leader.Callbacks{
		OnStartedLeading: func(leaderCtx context.Context) {
			a.logger.Info("became leader, starting background tasks")
			a.startBackgroundTasks(leaderCtx)
		},
		OnStoppedLeading: func() {
			a.logger.Info("lost leadership, stopping background tasks")
			if a.watcher != nil {
				a.watcher.StopWatchers()
			}
			if a.briefing != nil {
				a.briefing.StopScheduler()
			}
		},
		OnNewLeader: func(identity string) {
			a.logger.Info("new leader elected", zap.String("leader", identity))
		},
	}

	elector := leader.NewElector(clientset, leCfg, callbacks, a.logger)
	elector.Start(ctx)
}

func (a *App) shutdown() {
	// Stop bot
	a.bot.Stop()

	// Stop modules (includes watcher/briefing)
	shutdownCtx := context.Background()
	a.registry.StopAll(shutdownCtx)

	// Stop health server
	if err := a.health.Stop(shutdownCtx); err != nil {
		a.logger.Error("error stopping health server", zap.Error(err))
	}

	// Flush audit
	if err := a.audit.Close(); err != nil {
		a.logger.Error("error closing audit logger", zap.Error(err))
	}

	// Close Redis
	if a.redis != nil {
		if err := a.redis.Close(); err != nil {
			a.logger.Error("error closing redis", zap.Error(err))
		}
	}

	// Close storage
	if err := a.storage.Close(); err != nil {
		a.logger.Error("error closing storage", zap.Error(err))
	}

	a.logger.Info("telekube stopped")
}
