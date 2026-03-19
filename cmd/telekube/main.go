// Package main is the entrypoint for the Telekube bot.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/d9042n/telekube/internal/app"
	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/bot"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	approvalmod "github.com/d9042n/telekube/internal/module/approval"
	"github.com/d9042n/telekube/internal/module/briefing"
	argocdfmod "github.com/d9042n/telekube/internal/module/argocd"
	kubemod "github.com/d9042n/telekube/internal/module/kubernetes"
	"github.com/d9042n/telekube/internal/module/notify"
	"github.com/d9042n/telekube/internal/module/rbacmod"
	"github.com/d9042n/telekube/internal/module/watcher"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/d9042n/telekube/internal/storage"
	pgstore "github.com/d9042n/telekube/internal/storage/postgres"
	"github.com/d9042n/telekube/internal/storage/sqlite"
	pkgredis "github.com/d9042n/telekube/pkg/redis"
	"github.com/d9042n/telekube/pkg/health"
	"github.com/d9042n/telekube/pkg/logger"
	"github.com/d9042n/telekube/pkg/version"
	"go.uber.org/zap"
)

func main() {
	// Handle subcommands before loading full config.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup":
			output := "configs/config.yaml"
			if len(os.Args) > 2 {
				output = os.Args[2]
			}
			if err := runSetup(output); err != nil {
				fmt.Fprintf(os.Stderr, "setup error: %v\n", err)
				os.Exit(1)
			}
			return
		case "version":
			fmt.Printf("telekube %s\n", version.Version)
			return
		}
	}

	// 1. Load config
	cfg, err := app.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// 2. Init logger
	log, err := logger.New(cfg.Log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("starting telekube",
		zap.String("version", version.Version),
	)

	// 3. Init storage (supports sqlite and postgres backends)
	var store storage.Storage
	switch cfg.Storage.Backend {
	case "postgres":
		store, err = pgstore.New(pgstore.Config{
			DSN:             cfg.Storage.Postgres.DSN,
			MaxOpenConns:    cfg.Storage.Postgres.MaxOpenConns,
			MaxIdleConns:    cfg.Storage.Postgres.MaxIdleConns,
			ConnMaxLifetime: time.Duration(cfg.Storage.Postgres.ConnMaxLifetime) * time.Minute,
		}, log)
	default:
		store, err = sqlite.New(cfg.Storage.SQLite.Path)
	}
	if err != nil {
		log.Fatal("failed to init storage",
			zap.String("backend", cfg.Storage.Backend),
			zap.Error(err),
		)
	}

	// 4. Init Redis (optional)
	var redisClient *pkgredis.Client
	if cfg.Storage.Redis.Addr != "" {
		redisClient, err = pkgredis.New(pkgredis.Config{
			Addr:      cfg.Storage.Redis.Addr,
			Username:  cfg.Storage.Redis.Username,
			Password:  cfg.Storage.Redis.Password,
			DB:        cfg.Storage.Redis.DB,
			TLSEnable: cfg.Storage.Redis.TLSEnable,
			PoolSize:  cfg.Storage.Redis.PoolSize,
			OpTimeout: time.Duration(cfg.Storage.Redis.OpTimeout) * time.Millisecond,
		}, log)
		if err != nil {
			log.Warn("redis unavailable, continuing without cache",
				zap.Error(err),
			)
		} else {
			log.Info("redis connected")
		}
	}

	// 5. Init cluster manager
	clusterMgr := cluster.NewManager(cfg.Clusters, log)

	// 6. Init core services
	rbacEngine := rbac.NewEngine(cfg.RBAC.DefaultRole, store.RBAC(), cfg.Telegram.AdminIDs)
	auditLogger := audit.NewLogger(store.Audit(), log)
	userCtx := cluster.NewUserContext(clusterMgr)

	// 7. Init modules
	registry := module.NewRegistry(log)

	// Init approval module (Phase 4) — must be before other modules that use it
	var approvalRules []approvalmod.Rule
	for _, r := range cfg.Approval.Rules {
		approvalRules = append(approvalRules, approvalmod.Rule{
			Action:            r.Action,
			Clusters:          r.Clusters,
			RequiredApprovals: r.RequiredApprovals,
			ApproverRoles:     r.ApproverRoles,
		})
	}
	defaultExpiry := 30 * time.Minute
	if cfg.Approval.DefaultExpiry != "" {
		if parsed, parseErr := time.ParseDuration(cfg.Approval.DefaultExpiry); parseErr == nil {
			defaultExpiry = parsed
		}
	}
	approvalMgr := approvalmod.NewManager(cfg.Approval.Enabled, defaultExpiry, approvalRules, store.Approval(), log)
	approvalChecker := approvalmod.NewChecker(approvalMgr)
	approvalBotMod := approvalmod.NewBotModule(approvalMgr, log)

	if cfg.Modules.Kubernetes.Enabled {
		if regErr := registry.Register(kubemod.NewModule(clusterMgr, userCtx, rbacEngine, auditLogger, log,
			kubemod.WithApprovalChecker(approvalChecker),
		)); regErr != nil {
			log.Error("failed to register kubernetes module", zap.Error(regErr))
		}
	}

	// 8. Init bot
	teleBot, err := bot.New(cfg.Telegram, clusterMgr, store, rbacEngine, auditLogger, registry, log)
	if err != nil {
		log.Fatal("failed to create bot", zap.Error(err))
	}

	// Register approval bot module (must happen after bot is created)
	if regErr := registry.Register(approvalBotMod); regErr != nil {
		log.Error("failed to register approval module", zap.Error(regErr))
	}

	// Register RBAC management module (Phase 4 — /rbac command)
	rbacMod := rbacmod.NewModule(rbacEngine, store.Users(), auditLogger, log)
	if regErr := registry.Register(rbacMod); regErr != nil {
		log.Error("failed to register rbac module", zap.Error(regErr))
	}

	// Register notification preferences module (Phase 4 — /notify command)
	if cfg.Modules.Notify.Enabled {
		notifyMod := notify.NewModule(store.NotificationPrefs(), log)
		if regErr := registry.Register(notifyMod); regErr != nil {
			log.Error("failed to register notify module", zap.Error(regErr))
		}
	}

	// 9. Register watcher module
	var watcherMod *watcher.Module
	if cfg.Modules.Watcher.Enabled {
		notifier := bot.NewNotifier(teleBot)
		watcherMod = watcher.NewModule(clusterMgr, notifier, auditLogger, cfg.Telegram, log)

		// Convert custom alert rules from config to entity
		var customRules []entity.AlertRule
		for _, r := range cfg.Watcher.CustomRules {
			customRules = append(customRules, entity.AlertRule{
				Name:        r.Name,
				Description: r.Description,
				Severity:    r.Severity,
				Condition: entity.AlertCondition{
					Type:      r.Condition.Type,
					Threshold: r.Condition.Threshold,
					Window:    r.Condition.Window,
					Resource:  r.Condition.Resource,
				},
				Scope: entity.AlertScope{
					Clusters:   r.Scope.Clusters,
					Namespaces: r.Scope.Namespaces,
				},
				Notify: entity.AlertNotify{
					Chats:   r.Notify.Chats,
					Mention: r.Notify.Mention,
				},
			})
		}
		if len(customRules) > 0 {
			watcherMod.SetCustomRules(customRules)
		}

		if regErr := registry.Register(watcherMod); regErr != nil {
			log.Error("failed to register watcher module", zap.Error(regErr))
		}
	}

	// 10. Register ArgoCD module (Phase 3)
	if cfg.Modules.ArgoCD.Enabled && len(cfg.ArgoCD.Instances) > 0 {
		argoCDInstances, buildErr := argocdfmod.BuildInstances(cfg.ArgoCD, log)
		if buildErr != nil {
			log.Error("failed to build argocd instances", zap.Error(buildErr))
		} else {
			argoCDMod := argocdfmod.NewModule(
				argoCDInstances,
				rbacEngine,
				auditLogger,
				store.Freeze(),
				approvalChecker,
				log,
			)
			if regErr := registry.Register(argoCDMod); regErr != nil {
				log.Error("failed to register argocd module", zap.Error(regErr))
			} else {
				log.Info("argocd module registered",
					zap.Int("instances", len(argoCDInstances)),
				)
			}

			// Register ArgoCD watchers
			if watcherMod != nil {
				for _, inst := range argoCDInstances {
					watcherMod.AddArgoCDWatcher(watcher.ArgoCDWatcherConfig{
						InstanceName:         inst.Name(),
						Client:               inst.Client(),
						Clusters:             inst.Clusters(),
						NotifyOnHealthChange: true,
						NotifyOnSyncChange:   true,
					})
				}
			}
		}
	}

	// 11. Register briefing module
	var briefingMod *briefing.Module
	if cfg.Modules.Briefing.Enabled {
		notifier := bot.NewNotifier(teleBot)
		briefingMod = briefing.NewModule(
			clusterMgr,
			notifier,
			auditLogger,
			cfg.Telegram,
			briefing.Config{
				Enabled:  cfg.Modules.Briefing.Enabled,
				Schedule: cfg.Modules.Briefing.Schedule,
				Timezone: cfg.Modules.Briefing.Timezone,
			},
			log,
		)
		if regErr := registry.Register(briefingMod); regErr != nil {
			log.Error("failed to register briefing module", zap.Error(regErr))
		}
	}

	// 11. Init health server
	checker := health.NewChecker()
	checker.Register("storage", func(ctx context.Context) error {
		return store.Ping(ctx)
	})
	if redisClient != nil {
		checker.Register("redis", func(ctx context.Context) error {
			return redisClient.Ping(ctx)
		})
	}
	healthSrv := health.NewServer(cfg.Server.Port, checker)

	// 12. Run
	application := app.NewApp(
		teleBot, registry, healthSrv, auditLogger, store, log,
		app.WithRedis(redisClient),
		app.WithWatcher(watcherMod),
		app.WithBriefing(briefingMod),
		app.WithLeaderElection(cfg.LeaderElection, clusterMgr),
	)
	application.Run(context.Background())
}
