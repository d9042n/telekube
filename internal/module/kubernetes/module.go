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
	bot.Handle(&telebot.Btn{Unique: "k8s_ns"}, m.handleNamespaceSelect)
	bot.Handle(&telebot.Btn{Unique: "k8s_pod_detail"}, m.handlePodDetail)
	bot.Handle(&telebot.Btn{Unique: "k8s_pods_page"}, m.handlePodsPage)
	bot.Handle(&telebot.Btn{Unique: "k8s_pods_back"}, m.handlePodsBack)
	bot.Handle(&telebot.Btn{Unique: "k8s_pods_refresh"}, m.handlePodsRefresh)
	bot.Handle(&telebot.Btn{Unique: "k8s_logs"}, m.handleLogs)
	bot.Handle(&telebot.Btn{Unique: "k8s_logs_container"}, m.handleLogsContainer)
	bot.Handle(&telebot.Btn{Unique: "k8s_logs_more"}, m.handleLogsMore)
	bot.Handle(&telebot.Btn{Unique: "k8s_logs_prev"}, m.handleLogsPrevious)
	bot.Handle(&telebot.Btn{Unique: "k8s_events"}, m.handleEvents)
	bot.Handle(&telebot.Btn{Unique: "k8s_events_refresh"}, m.handleEventsRefresh)
	bot.Handle(&telebot.Btn{Unique: "k8s_restart"}, m.handleRestart)
	bot.Handle(&telebot.Btn{Unique: "k8s_restart_confirm"}, m.handleRestartConfirm)
	bot.Handle(&telebot.Btn{Unique: "k8s_restart_cancel"}, m.handleRestartCancel)

	// Phase 2 Callbacks — top (metrics)
	bot.Handle(&telebot.Btn{Unique: "k8s_top_ns"}, m.handleTopNamespaceSelect)
	bot.Handle(&telebot.Btn{Unique: "k8s_top_page"}, m.handleTopPage)
	bot.Handle(&telebot.Btn{Unique: "k8s_top_refresh"}, m.handleTopRefresh)
	bot.Handle(&telebot.Btn{Unique: "k8s_top_nodes"}, m.handleTopNodesCallback)
	bot.Handle(&telebot.Btn{Unique: "k8s_top_nodes_refresh"}, m.handleTopNodesRefresh)
	bot.Handle(&telebot.Btn{Unique: "k8s_top_back"}, m.handleTopBack)
	bot.Handle(&telebot.Btn{Unique: "k8s_top_info"}, noopCallback)

	// Phase 2 Callbacks — scale
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_ns"}, m.handleScaleNamespaceSelect)
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_detail"}, m.handleScaleDetail)
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_set"}, m.handleScaleSet)
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_confirm"}, m.handleScaleConfirm)
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_cancel"}, m.handleScaleCancel)
	bot.Handle(&telebot.Btn{Unique: "k8s_scale_back"}, m.handleScaleBack)

	// Phase 2 Callbacks — nodes
	bot.Handle(&telebot.Btn{Unique: "k8s_node_detail"}, m.handleNodeDetail)
	bot.Handle(&telebot.Btn{Unique: "k8s_nodes_refresh"}, m.handleNodesRefresh)
	bot.Handle(&telebot.Btn{Unique: "k8s_node_cordon"}, m.handleNodeCordon)
	bot.Handle(&telebot.Btn{Unique: "k8s_node_cordon_confirm"}, m.handleNodeCordonConfirm)
	bot.Handle(&telebot.Btn{Unique: "k8s_node_uncordon"}, m.handleNodeUncordon)
	bot.Handle(&telebot.Btn{Unique: "k8s_node_uncordon_confirm"}, m.handleNodeUncordonConfirm)
	bot.Handle(&telebot.Btn{Unique: "k8s_node_drain"}, m.handleNodeDrain)
	bot.Handle(&telebot.Btn{Unique: "k8s_node_drain_confirm"}, m.handleNodeDrainConfirm)
	bot.Handle(&telebot.Btn{Unique: "k8s_node_top_pods"}, m.handleNodeTopPods)
	bot.Handle(&telebot.Btn{Unique: "k8s_nodes_back"}, m.handleNodesBack)

	// Phase 2 Callbacks — quota
	bot.Handle(&telebot.Btn{Unique: "k8s_quota_ns"}, m.handleQuotaNamespaceSelect)
	bot.Handle(&telebot.Btn{Unique: "k8s_quota_refresh"}, m.handleQuotaRefresh)
	bot.Handle(&telebot.Btn{Unique: "k8s_quota_back"}, m.handleQuotaBack)

	// Noop callbacks for info buttons
	bot.Handle(&telebot.Btn{Unique: "k8s_pods_info"}, noopCallback)

	// Namespace list callbacks
	bot.Handle(&telebot.Btn{Unique: "k8s_namespaces_refresh"}, m.handleNamespacesRefresh)

	// Deployment list callbacks
	bot.Handle(&telebot.Btn{Unique: "k8s_deploys_ns"}, m.handleDeploysNamespaceSelect)
	bot.Handle(&telebot.Btn{Unique: "k8s_deploys_refresh"}, m.handleDeploysRefresh)

	// CronJob list callbacks
	bot.Handle(&telebot.Btn{Unique: "k8s_cronjobs_ns"}, m.handleCronJobsNamespaceSelect)
	bot.Handle(&telebot.Btn{Unique: "k8s_cronjobs_refresh"}, m.handleCronJobsRefresh)
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
