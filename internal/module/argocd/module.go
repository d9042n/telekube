// Package argocd implements the ArgoCD integration module for Telekube.
package argocd

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/bot/keyboard"
	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/module/approval"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/d9042n/telekube/internal/storage"
	pkgargocd "github.com/d9042n/telekube/pkg/argocd"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// instanceClient pairs an ArgoCD API client with its configuration.
type instanceClient struct {
	cfg    config.ArgoCDInstanceConfig
	client pkgargocd.Client
}

// InstanceInfo is a public view of an ArgoCD instance for external wiring.
type InstanceInfo struct {
	name     string
	client   pkgargocd.Client
	clusters []string
}

// Name returns the instance name.
func (i *InstanceInfo) Name() string { return i.name }

// Client returns the API client.
func (i *InstanceInfo) Client() pkgargocd.Client { return i.client }

// Clusters returns the associated Kubernetes cluster names.
func (i *InstanceInfo) Clusters() []string { return i.clusters }

// NewInstanceInfo creates an InstanceInfo (used in tests and external wiring).
func NewInstanceInfo(name string, client pkgargocd.Client, clusters []string) *InstanceInfo {
	return &InstanceInfo{name: name, client: client, clusters: clusters}
}

// Module implements the ArgoCD operations feature.
type Module struct {
	instances []instanceClient
	rbac      rbac.Engine
	audit     audit.Logger
	freeze    storage.FreezeRepository
	approval  approval.Checker
	kb        *keyboard.Builder
	logger    *zap.Logger
	mu        sync.RWMutex
	healthy   bool
}

// NewModule creates a new ArgoCD module from a list of public InstanceInfo.
func NewModule(
	infos []*InstanceInfo,
	rbacEngine rbac.Engine,
	auditLogger audit.Logger,
	freezeRepo storage.FreezeRepository,
	approvalChecker approval.Checker,
	logger *zap.Logger,
) *Module {
	instances := make([]instanceClient, 0, len(infos))
	for _, info := range infos {
		instances = append(instances, instanceClient{
			cfg: config.ArgoCDInstanceConfig{
				Name:     info.name,
				Clusters: info.clusters,
			},
			client: info.client,
		})
	}
	return &Module{
		instances: instances,
		rbac:      rbacEngine,
		audit:     auditLogger,
		freeze:    freezeRepo,
		approval:  approvalChecker,
		kb:        keyboard.NewBuilder(),
		logger:    logger,
		healthy:   true,
	}
}

// BuildInstances constructs InstanceInfo list from config.
func BuildInstances(cfg config.ArgoCDConfig, logger *zap.Logger) ([]*InstanceInfo, error) {
	timeout := 30 * time.Second
	if cfg.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Timeout); err == nil {
			timeout = d
		}
	}

	results := make([]*InstanceInfo, 0, len(cfg.Instances))
	for _, ic := range cfg.Instances {
		var auth pkgargocd.AuthProvider
		switch ic.Auth.Type {
		case "oauth":
			auth = pkgargocd.NewOAuthAuth(ic.Auth.TokenURL, ic.Auth.ClientID, ic.Auth.ClientSecret, &http.Client{Timeout: timeout})
		default:
			auth = pkgargocd.NewTokenAuth(ic.Auth.Token)
		}

		client := pkgargocd.NewClient(pkgargocd.ClientConfig{
			BaseURL:  ic.URL,
			Auth:     auth,
			Insecure: cfg.Insecure,
			Timeout:  timeout,
			Logger:   logger,
		})

		results = append(results, &InstanceInfo{
			name:     ic.Name,
			client:   client,
			clusters: ic.Clusters,
		})
		logger.Info("argocd instance configured",
			zap.String("name", ic.Name),
			zap.String("url", ic.URL),
		)
	}
	return results, nil
}

func (m *Module) Name() string        { return "argocd" }
func (m *Module) Description() string { return "ArgoCD GitOps management" }

func (m *Module) Register(bot *telebot.Bot, _ *telebot.Group) {
	// Commands
	bot.Handle("/apps", m.handleApps)
	bot.Handle("/dashboard", m.handleDashboard)
	bot.Handle("/freeze", m.handleFreeze)

	// App list callbacks
	bot.Handle(&telebot.Btn{Unique: "argo_app_detail"}, m.handleAppDetail)
	bot.Handle(&telebot.Btn{Unique: "argo_apps_refresh"}, m.handleAppsRefresh)
	bot.Handle(&telebot.Btn{Unique: "argo_apps_back"}, m.handleAppsBack)
	bot.Handle(&telebot.Btn{Unique: "argo_inst_select"}, m.handleInstanceSelect)

	// Sync callbacks
	bot.Handle(&telebot.Btn{Unique: "argo_sync"}, m.handleSync)
	bot.Handle(&telebot.Btn{Unique: "argo_sync_now"}, m.handleSyncNow)
	bot.Handle(&telebot.Btn{Unique: "argo_sync_prune"}, m.handleSyncPrune)
	bot.Handle(&telebot.Btn{Unique: "argo_sync_force"}, m.handleSyncForce)
	bot.Handle(&telebot.Btn{Unique: "argo_sync_confirm"}, m.handleSyncConfirm)
	bot.Handle(&telebot.Btn{Unique: "argo_sync_cancel"}, m.handleSyncCancel)

	// Rollback callbacks
	bot.Handle(&telebot.Btn{Unique: "argo_rollback"}, m.handleRollback)
	bot.Handle(&telebot.Btn{Unique: "argo_rollback_select"}, m.handleRollbackSelect)
	bot.Handle(&telebot.Btn{Unique: "argo_rollback_confirm"}, m.handleRollbackConfirm)
	bot.Handle(&telebot.Btn{Unique: "argo_rollback_cancel"}, m.handleRollbackCancel)

	// Dashboard callbacks
	bot.Handle(&telebot.Btn{Unique: "argo_dash_refresh"}, m.handleDashboardRefresh)
	bot.Handle(&telebot.Btn{Unique: "argo_dash_outofsync"}, m.handleDashboardOutOfSync)
	bot.Handle(&telebot.Btn{Unique: "argo_dash_degraded"}, m.handleDashboardDegraded)

	// Freeze callbacks
	bot.Handle(&telebot.Btn{Unique: "argo_freeze_create"}, m.handleFreezeCreate)
	bot.Handle(&telebot.Btn{Unique: "argo_freeze_scope"}, m.handleFreezeScope)
	bot.Handle(&telebot.Btn{Unique: "argo_freeze_duration"}, m.handleFreezeDuration)
	bot.Handle(&telebot.Btn{Unique: "argo_freeze_confirm"}, m.handleFreezeConfirm)
	bot.Handle(&telebot.Btn{Unique: "argo_freeze_thaw"}, m.handleFreezeThaw)
	bot.Handle(&telebot.Btn{Unique: "argo_freeze_history"}, m.handleFreezeHistory)

	// Noop for info buttons
	bot.Handle(&telebot.Btn{Unique: "argo_noop"}, noopCallback)
}

func (m *Module) Start(ctx context.Context) error {
	if len(m.instances) == 0 {
		m.logger.Warn("argocd module: no instances configured")
		return nil
	}

	// Verify connectivity to each instance
	for _, inst := range m.instances {
		pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := inst.client.Ping(pingCtx)
		cancel()
		if err != nil {
			m.logger.Warn("argocd instance unreachable at startup",
				zap.String("name", inst.cfg.Name),
				zap.Error(err),
			)
		} else {
			m.logger.Info("argocd instance connected",
				zap.String("name", inst.cfg.Name),
			)
		}
	}
	m.logger.Info("argocd module started")
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	m.logger.Info("argocd module stopped")
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
	return []module.CommandInfo{
		{
			Command:     "/apps",
			Description: "List ArgoCD applications with status",
			Permission:  rbac.PermArgoCDAppsList,
			ChatType:    "all",
		},
		{
			Command:     "/dashboard",
			Description: "GitOps status dashboard across all ArgoCD instances",
			Permission:  rbac.PermArgoCDAppsList,
			ChatType:    "all",
		},
		{
			Command:     "/freeze",
			Description: "Manage deployment freeze (block sync/rollback)",
			Permission:  rbac.PermArgoCDFreezeManage,
			ChatType:    "all",
		},
	}
}

// noopCallback handles info-only buttons.
func noopCallback(c telebot.Context) error {
	return c.Respond(&telebot.CallbackResponse{})
}

// getInstanceByName finds an instance client by name.
func (m *Module) getInstanceByName(name string) (*instanceClient, error) {
	for i := range m.instances {
		if m.instances[i].cfg.Name == name {
			return &m.instances[i], nil
		}
	}
	return nil, fmt.Errorf("argocd instance %q not found", name)
}

// getDefaultInstance returns the first configured instance (most common case).
func (m *Module) getDefaultInstance() (*instanceClient, error) {
	if len(m.instances) == 0 {
		return nil, fmt.Errorf("no ArgoCD instances configured")
	}
	return &m.instances[0], nil
}

// syncStatusEmoji maps ArgoCD sync/health status to an emoji.
func syncStatusEmoji(syncStatus, healthStatus string) string {
	if syncStatus == "OutOfSync" {
		return "🟡"
	}
	if healthStatus == "Degraded" || healthStatus == "Missing" {
		return "🔴"
	}
	if healthStatus == "Progressing" {
		return "⚪"
	}
	if syncStatus == "Synced" && healthStatus == "Healthy" {
		return "✅"
	}
	return "⚪"
}

// resourceStatusEmoji maps resource status to an emoji.
func resourceStatusEmoji(status, health string) string {
	if status == "OutOfSync" {
		return "🟡"
	}
	if health == "Degraded" || health == "Missing" {
		return "🔴"
	}
	if status == "Synced" {
		return "✅"
	}
	return "⚪"
}

// shortRev returns the first 7 characters of a git revision.
func shortRev(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}
