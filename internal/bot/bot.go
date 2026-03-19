// Package bot provides the Telegram bot wrapper and lifecycle management.
package bot

import (
	"fmt"
	"time"

	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/bot/handler"
	"github.com/d9042n/telekube/internal/bot/keyboard"
	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/d9042n/telekube/internal/storage"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// Bot wraps the telebot instance with application dependencies.
type Bot struct {
	tele        *telebot.Bot
	modules     *module.Registry
	cluster     cluster.Manager
	userCtx     *cluster.UserContext
	storage     storage.Storage
	rbac        rbac.Engine
	audit       audit.Logger
	logger      *zap.Logger
	cfg         config.TelegramConfig
}

// New creates a new Telegram bot.
func New(
	cfg config.TelegramConfig,
	clusterMgr cluster.Manager,
	store storage.Storage,
	rbacEngine rbac.Engine,
	auditLogger audit.Logger,
	registry *module.Registry,
	logger *zap.Logger,
) (*Bot, error) {
	return NewWithURL(cfg, clusterMgr, store, rbacEngine, auditLogger, registry, logger, "")
}

// NewWithURL creates a new Telegram bot with a custom API base URL.
// Pass an empty string for apiURL to use the default Telegram API.
// This is intended for testing with a fake Telegram server.
func NewWithURL(
	cfg config.TelegramConfig,
	clusterMgr cluster.Manager,
	store storage.Storage,
	rbacEngine rbac.Engine,
	auditLogger audit.Logger,
	registry *module.Registry,
	logger *zap.Logger,
	apiURL string,
) (*Bot, error) {
	pollerTimeout := 10 * time.Second
	if apiURL != "" {
		// Fast polling for tests — no real long-poll needed.
		pollerTimeout = 0
	}
	settings := telebot.Settings{
		Token:  cfg.Token,
		URL:    apiURL,
		Poller: &telebot.LongPoller{Timeout: pollerTimeout},
	}

	if cfg.WebhookURL != "" {
		settings.Poller = &telebot.Webhook{
			Endpoint: &telebot.WebhookEndpoint{PublicURL: cfg.WebhookURL},
		}
	}

	tele, err := telebot.NewBot(settings)
	if err != nil {
		return nil, fmt.Errorf("creating telegram bot: %w", err)
	}

	userCtx := cluster.NewUserContext(clusterMgr)

	b := &Bot{
		tele:    tele,
		modules: registry,
		cluster: clusterMgr,
		userCtx: userCtx,
		storage: store,
		rbac:    rbacEngine,
		audit:   auditLogger,
		logger:  logger,
		cfg:     cfg,
	}

	b.registerMiddleware()
	b.registerHandlers()

	return b, nil
}

func (b *Bot) registerMiddleware() {
	b.tele.Use(
		middleware.Recovery(b.logger),
		middleware.Auth(b.storage, b.cfg, b.logger),
		middleware.RateLimit(b.cfg.RateLimit),
		middleware.Audit(b.audit),
	)
}

func (b *Bot) registerHandlers() {
	kbBuilder := keyboard.NewBuilder()

	// Base commands
	b.tele.Handle("/start", handler.Start(b.cluster, b.userCtx, b.rbac, kbBuilder))
	b.tele.Handle("/help", handler.Help(b.modules, b.rbac))
	b.tele.Handle("/clusters", handler.Clusters(b.cluster, b.userCtx, kbBuilder))
	b.tele.Handle("/audit", handler.AuditLog(b.audit, b.rbac))

	// Cluster selection callback
	b.tele.Handle(&telebot.Btn{Unique: "cluster_select"}, handler.ClusterSelect(b.cluster, b.userCtx))

	// Register module commands
	b.modules.RegisterAll(b.tele)
}

// Start begins the bot polling/webhook loop.
func (b *Bot) Start() {
	b.logger.Info("starting telegram bot")
	b.tele.Start()
}

// Stop stops the bot.
func (b *Bot) Stop() {
	b.logger.Info("stopping telegram bot")
	b.tele.Stop()
}
