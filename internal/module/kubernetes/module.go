// Package kubernetes implements the Kubernetes operations module.
package kubernetes

import (
	"context"
	"sync"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/bot/keyboard"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/module/approval"
	"github.com/d9042n/telekube/internal/rbac"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// Module implements the Kubernetes operations feature.
type Module struct {
	cluster  cluster.Manager
	userCtx  *cluster.UserContext
	rbac     rbac.Engine
	audit    audit.Logger
	approval approval.Checker
	kb       *keyboard.Builder
	logger   *zap.Logger
	nsCache  *namespaceCache
	mu       sync.RWMutex
	healthy  bool
}

// NewModule creates a new Kubernetes module.
func NewModule(
	clusterMgr cluster.Manager,
	userCtx *cluster.UserContext,
	rbacEngine rbac.Engine,
	auditLogger audit.Logger,
	logger *zap.Logger,
	opts ...ModuleOption,
) *Module {
	m := &Module{
		cluster: clusterMgr,
		userCtx: userCtx,
		rbac:    rbacEngine,
		audit:   auditLogger,
		kb:      keyboard.NewBuilder(),
		logger:  logger,
		nsCache: newNamespaceCache(),
		healthy: true,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// ModuleOption configures the Module.
type ModuleOption func(*Module)

// WithApprovalChecker sets the approval checker for gating dangerous operations.
func WithApprovalChecker(checker approval.Checker) ModuleOption {
	return func(m *Module) {
		m.approval = checker
	}
}

func (m *Module) Name() string        { return "kubernetes" }
func (m *Module) Description() string { return "Kubernetes cluster operations" }

func (m *Module) Register(bot *telebot.Bot, group *telebot.Group) {
	// Phase 1 Commands
	bot.Handle("/pods", m.handlePods)
	bot.Handle("/logs", m.handleLogsCommand)
	bot.Handle("/events", m.handleEventsCommand)

	// Phase 2 Commands
	bot.Handle("/top", m.handleTop)
	bot.Handle("/scale", m.handleScale)
	bot.Handle("/nodes", m.handleNodes)
	bot.Handle("/quota", m.handleQuota)
	bot.Handle("/restart", m.handleRestartCommand)
	bot.Handle("/namespaces", m.handleNamespacesCommand)
	bot.Handle("/deploys", m.handleDeploysCommand)
	bot.Handle("/cronjobs", m.handleCronJobsCommand)

	// Phase 1 Callbacks — pods
	bot.Handle(&telebot.Btn{Unique: "k8s_ns"}, m.withResolve(m.handleNamespaceSelect))
	bot.Handle(&telebot.Btn{Unique: "k8s_pod_detail"}, m.withResolve(m.handlePodDetail))
	bot.Handle(&telebot.Btn{Unique: "k8s_pods_page"}, m.withResolve(m.handlePodsPage))
	bot.Handle(&telebot.Btn{Unique: "k8s_pods_back"}, m.withResolve(m.handlePodsBack))
	bot.Handle(&telebot.Btn{Unique: "k8s_pods_refresh"}, m.withResolve(m.handlePodsRefresh))
	bot.Handle(&telebot.Btn{Unique: "k8s_logs"}, m.withResolve(m.handleLogs))
	bot.Handle(&telebot.Btn{Unique: "k8s_logs_container"}, m.withResolve(m.handleLogsContainer))
	bot.Handle(&telebot.Btn{Unique: "k8s_logs_more"}, m.withResolve(m.handleLogsMore))
	bot.Handle(&telebot.Btn{Unique: "k8s_logs_prev"}, m.withResolve(m.handleLogsPrevious))
	bot.Handle(&telebot.Btn{Unique: "k8s_events"}, m.withResolve(m.handleEvents))
	bot.Handle(&telebot.Btn{Unique: "k8s_events_refresh"}, m.withResolve(m.handleEventsRefresh))
	bot.Handle(&telebot.Btn{Unique: "k8s_restart"}, m.withResolve(m.handleRestart))
	bot.Handle(&telebot.Btn{Unique: "k8s_restart_confirm"}, m.withResolve(m.handleRestartConfirm))
	bot.Handle(&telebot.Btn{Unique: "k8s_restart_cancel"}, m.withResolve(m.handleRestartCancel))

	// Phase 2 Callbacks — top (metrics)
	bot.Handle(&telebot.Btn{Unique: "k8s_top_ns"}, m.withResolve(m.handleTopNamespaceSelect))
	bot.Handle(&telebot.Btn{Unique: "k8s_top_page"}, m.withResolve(m.handleTopPage))
	bot.Handle(&telebot.Btn{Unique: "k8s_top_refresh"}, m.withResolve(m.handleTopRefresh))
	bot.Handle(&telebot.Btn{Unique: "k8s_top_nodes"}, m.withResolve(m.handleTopNodesCallback))
	bot.Handle(&telebot.Btn{Unique: "k8s_top_nodes_refresh"}, m.withResolve(m.handleTopNodesRefresh))
	bot.Handle(&telebot.Btn{Unique: "k8s_top_back"}, m.withResolve(m.handleTopBack))
	bot.Handle(&telebot.Btn{Unique: "k8s_top_info"}, noopCallback)

	// Phase 2 Callbacks — scale
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_ns"}, m.withResolve(m.handleScaleNamespaceSelect))
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_detail"}, m.withResolve(m.handleScaleDetail))
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_set"}, m.withResolve(m.handleScaleSet))
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_confirm"}, m.withResolve(m.handleScaleConfirm))
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_cancel"}, m.withResolve(m.handleScaleCancel))
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_back"}, m.withResolve(m.handleScaleBack))

	// Phase 2 Callbacks — nodes
	bot.Handle(&telebot.Btn{Unique: "k8s_node_detail"}, m.withResolve(m.handleNodeDetail))
	bot.Handle(&telebot.Btn{Unique: "k8s_nodes_refresh"}, m.withResolve(m.handleNodesRefresh))
	bot.Handle(&telebot.Btn{Unique: "k8s_node_cordon"}, m.withResolve(m.handleNodeCordon))
	bot.Handle(&telebot.Btn{Unique: "k8s_node_cordon_confirm"}, m.withResolve(m.handleNodeCordonConfirm))
	bot.Handle(&telebot.Btn{Unique: "k8s_node_uncordon"}, m.withResolve(m.handleNodeUncordon))
	bot.Handle(&telebot.Btn{Unique: "k8s_node_uncordon_confirm"}, m.withResolve(m.handleNodeUncordonConfirm))
	bot.Handle(&telebot.Btn{Unique: "k8s_node_drain"}, m.withResolve(m.handleNodeDrain))
	bot.Handle(&telebot.Btn{Unique: "k8s_node_drain_confirm"}, m.withResolve(m.handleNodeDrainConfirm))
	bot.Handle(&telebot.Btn{Unique: "k8s_node_top_pods"}, m.withResolve(m.handleNodeTopPods))
	bot.Handle(&telebot.Btn{Unique: "k8s_nodes_back"}, m.withResolve(m.handleNodesBack))

	// Phase 2 Callbacks — quota
	bot.Handle(&telebot.Btn{Unique: "k8s_quota_ns"}, m.withResolve(m.handleQuotaNamespaceSelect))
	bot.Handle(&telebot.Btn{Unique: "k8s_quota_refresh"}, m.withResolve(m.handleQuotaRefresh))
	bot.Handle(&telebot.Btn{Unique: "k8s_quota_back"}, m.withResolve(m.handleQuotaBack))

	// Noop callbacks for info buttons
	bot.Handle(&telebot.Btn{Unique: "k8s_pods_info"}, noopCallback)

	// Namespace list callbacks
	bot.Handle(&telebot.Btn{Unique: "k8s_namespaces_refresh"}, m.withResolve(m.handleNamespacesRefresh))

	// Deployment list callbacks
	bot.Handle(&telebot.Btn{Unique: "k8s_deploys_ns"}, m.withResolve(m.handleDeploysNamespaceSelect))
	bot.Handle(&telebot.Btn{Unique: "k8s_deploys_refresh"}, m.withResolve(m.handleDeploysRefresh))

	// CronJob list callbacks
	bot.Handle(&telebot.Btn{Unique: "k8s_cronjobs_ns"}, m.withResolve(m.handleCronJobsNamespaceSelect))
	bot.Handle(&telebot.Btn{Unique: "k8s_cronjobs_refresh"}, m.withResolve(m.handleCronJobsRefresh))
}

// withResolve wraps a handler to auto-resolve shortened callback data keys
// back to full strings before the handler runs.
func (m *Module) withResolve(handler telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		if cb := c.Callback(); cb != nil && cb.Data != "" {
			cb.Data = m.kb.Resolve(cb.Data)
		}
		return handler(c)
	}
}

// sd stores callback data, returning a short hash key if it exceeds the safe size.
// Short alias for m.kb.StoreData() used across handler files.
func (m *Module) sd(data string) string {
	return m.kb.StoreData(data)
}

func (m *Module) Start(ctx context.Context) error {
	m.logger.Info("kubernetes module started")
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	m.logger.Info("kubernetes module stopped")
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
		// Phase 1
		{
			Command:     "/pods",
			Description: "List pods in a namespace",
			Permission:  rbac.PermKubernetesPodsList,
			ChatType:    "all",
		},
		{
			Command:     "/logs <pod>",
			Description: "View pod logs",
			Permission:  rbac.PermKubernetesPodsLogs,
			ChatType:    "all",
		},
		{
			Command:     "/events <pod>",
			Description: "View pod events",
			Permission:  rbac.PermKubernetesPodsEvents,
			ChatType:    "all",
		},
		// Phase 2
		{
			Command:     "/top",
			Description: "Show pod/node resource usage",
			Permission:  rbac.PermKubernetesMetricsView,
			ChatType:    "all",
		},
		{
			Command:     "/top nodes",
			Description: "Show node resource usage",
			Permission:  rbac.PermKubernetesMetricsView,
			ChatType:    "all",
		},
		{
			Command:     "/scale",
			Description: "Scale deployment/statefulset replicas",
			Permission:  rbac.PermKubernetesDeploymentsScale,
			ChatType:    "all",
		},
		{
			Command:     "/nodes",
			Description: "List and manage cluster nodes",
			Permission:  rbac.PermKubernetesNodesView,
			ChatType:    "all",
		},
		{
			Command:     "/quota",
			Description: "Show namespace resource quotas",
			Permission:  rbac.PermKubernetesQuotaView,
			ChatType:    "all",
		},
		{
			Command:     "/restart <pod>",
			Description: "Restart (delete) a pod",
			Permission:  rbac.PermKubernetesPodsRestart,
			ChatType:    "all",
		},
		{
			Command:     "/namespaces",
			Description: "List all namespaces",
			Permission:  rbac.PermKubernetesNamespacesList,
			ChatType:    "all",
		},
		{
			Command:     "/deploys",
			Description: "List deployments in a namespace",
			Permission:  rbac.PermKubernetesDeploymentsList,
			ChatType:    "all",
		},
		{
			Command:     "/cronjobs",
			Description: "List CronJob status",
			Permission:  rbac.PermKubernetesCronJobsList,
			ChatType:    "all",
		},
	}
}

// noopCallback does nothing — used for info-only buttons.
func noopCallback(c telebot.Context) error {
	return c.Respond(&telebot.CallbackResponse{})
}
