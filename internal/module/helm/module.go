// Package helm implements the Helm release management module for Telekube.
package helm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/bot/keyboard"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/rbac"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	helmrelease "helm.sh/helm/v3/pkg/release"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sClient "k8s.io/client-go/kubernetes"
	k8s "k8s.io/client-go/rest"
)

// ClusterClient pairs a cluster name with its Kubernetes REST config.
type ClusterClient struct {
	Name       string
	Kubeconfig *k8s.Config
}

// Module implements the Helm release management feature.
type Module struct {
	clusters []ClusterClient
	rbac     rbac.Engine
	audit    audit.Logger
	logger   *zap.Logger
	cbs      *keyboard.CallbackStore

	// newClient builds a ReleaseClient for a given cluster REST config +
	// namespace. Defaults to DefaultClientFactory; overridden in tests.
	newClient ClientFactory
}

// NewModule creates a new Helm module using the production Helm SDK factory.
func NewModule(clusters []ClusterClient, rbacEngine rbac.Engine, auditLogger audit.Logger, logger *zap.Logger) *Module {
	return &Module{
		clusters:  clusters,
		rbac:      rbacEngine,
		audit:     auditLogger,
		logger:    logger,
		cbs:       keyboard.NewCallbackStore(),
		newClient: DefaultClientFactory,
	}
}

// newModuleWithFactory is used exclusively in tests to inject a stub factory.
func newModuleWithFactory(clusters []ClusterClient, rbacEngine rbac.Engine, auditLogger audit.Logger, logger *zap.Logger, factory ClientFactory) *Module {
	m := NewModule(clusters, rbacEngine, auditLogger, logger)
	m.newClient = factory
	return m
}

// SetClientFactory replaces the Helm client factory. This is intended for
// testing so that E2E harnesses can inject a fake ReleaseClient.
func (m *Module) SetClientFactory(f ClientFactory) {
	m.newClient = f
}

func (m *Module) Name() string        { return "helm" }
func (m *Module) Description() string { return "Helm release management" }

func (m *Module) Register(bot *telebot.Bot, _ *telebot.Group) {
	bot.Handle("/helm", m.handleHelm)
	bot.Handle(&telebot.Btn{Unique: "helm_ns_select"}, m.resolve(m.handleNamespaceSelect))
	bot.Handle(&telebot.Btn{Unique: "helm_release_detail"}, m.resolve(m.handleReleaseDetail))
	bot.Handle(&telebot.Btn{Unique: "helm_rollback_select"}, m.resolve(m.handleRollbackSelect))
	bot.Handle(&telebot.Btn{Unique: "helm_rollback_confirm"}, m.resolve(m.handleRollbackConfirm))
	bot.Handle(&telebot.Btn{Unique: "helm_rollback_cancel"}, m.handleRollbackCancel)
	bot.Handle(&telebot.Btn{Unique: "helm_refresh"}, m.resolve(m.handleRefresh))
}

// resolve is middleware that resolves shortened callback data back to full strings.
func (m *Module) resolve(handler telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		if cb := c.Callback(); cb != nil && cb.Data != "" {
			cb.Data = m.cbs.Resolve(cb.Data)
		}
		return handler(c)
	}
}

// sd stores callback data, returning a short hash if it exceeds safe size.
func (m *Module) sd(data string) string {
	return m.cbs.Store(data)
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("helm module started", zap.Int("clusters", len(m.clusters)))
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }

func (m *Module) Health() entity.HealthStatus { return entity.HealthStatusHealthy }

func (m *Module) Commands() []module.CommandInfo {
	return []module.CommandInfo{
		{
			Command:     "/helm",
			Description: "List and manage Helm releases",
			Permission:  rbac.PermHelmReleaseslist,
			ChatType:    "all",
		},
	}
}

func (m *Module) handleHelm(c telebot.Context) error {
	ok, err := m.rbac.HasPermission(context.Background(), c.Sender().ID, rbac.PermHelmReleaseslist)
	if err != nil || !ok {
		return c.Send("🚫 Insufficient permissions")
	}

	if len(m.clusters) == 0 {
		return c.Send("⚎ No clusters configured for Helm")
	}

	// Use first cluster.
	cluster := m.clusters[0]
	menu := &telebot.ReplyMarkup{}

	// Always offer "All Namespaces" first
	rows := []telebot.Row{
		menu.Row(menu.Data("[All Namespaces]", "helm_ns_select", m.sd(cluster.Name+"|"))),
	}

	// Dynamically query namespaces from cluster using the REST config
	namespaces := m.listClusterNamespaces(cluster)
	for _, ns := range namespaces {
		rows = append(rows, menu.Row(menu.Data(ns, "helm_ns_select", m.sd(cluster.Name+"|"+ns))))
	}

	menu.Inline(rows...)
	return c.Send("⎈ Select namespace for Helm releases:", menu)
}

// listClusterNamespaces dynamically queries namespaces from a cluster.
// Falls back to common defaults if the API call fails.
func (m *Module) listClusterNamespaces(cluster ClusterClient) []string {
	if cluster.Kubeconfig == nil {
		return []string{"default", "production", "staging", "kube-system"}
	}

	cs, err := k8sClient.NewForConfig(cluster.Kubeconfig)
	if err != nil {
		m.logger.Debug("failed to create clientset for namespace list, using defaults", zap.Error(err))
		return []string{"default", "production", "staging", "kube-system"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		m.logger.Debug("failed to list namespaces for helm, using defaults", zap.Error(err))
		return []string{"default", "production", "staging", "kube-system"}
	}

	names := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		names = append(names, ns.Name)
	}
	return names
}

func (m *Module) handleNamespaceSelect(c telebot.Context) error {
	_ = c.Respond(&telebot.CallbackResponse{})
	data := c.Callback().Data
	parts := strings.SplitN(data, "|", 2)
	if len(parts) != 2 {
		return c.Send("❌ Invalid selection")
	}
	clusterName, namespace := parts[0], parts[1]

	releases, err := m.listReleases(clusterName, namespace)
	if err != nil {
		m.logger.Error("listing helm releases", zap.Error(err))
		return c.Send(fmt.Sprintf("❌ Error listing releases: %v", err))
	}

	text := formatReleaseList(clusterName, namespace, releases)
	menu := &telebot.ReplyMarkup{}

	rows := make([]telebot.Row, 0, len(releases)+1)
	for _, rel := range releases {
		btn := menu.Data(fmt.Sprintf("%s (%s)", rel.Name, rel.Chart.Metadata.AppVersion),
			"helm_release_detail", m.sd(fmt.Sprintf("%s|%s|%s", clusterName, namespace, rel.Name)))
		rows = append(rows, menu.Row(btn))
	}
	rows = append(rows, menu.Row(
		menu.Data("🔄 Refresh", "helm_refresh", m.sd(clusterName+"|"+namespace)),
	))
	menu.Inline(rows...)

	return c.Edit(text, menu)
}

func (m *Module) handleReleaseDetail(c telebot.Context) error {
	_ = c.Respond(&telebot.CallbackResponse{})
	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) != 3 {
		return c.Send("❌ Invalid selection")
	}
	clusterName, namespace, releaseName := parts[0], parts[1], parts[2]

	rel, history, err := m.getReleaseDetail(clusterName, namespace, releaseName)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Error: %v", err))
	}

	text := formatReleaseDetail(rel, history)
	menu := &telebot.ReplyMarkup{}
	rollbackBtn := menu.Data("⏪ Rollback", "helm_rollback_select", m.sd(fmt.Sprintf("%s|%s|%s", clusterName, namespace, releaseName)))
	backBtn := menu.Data("◀️ Back", "helm_ns_select", m.sd(clusterName+"|"+namespace))
	menu.Inline(menu.Row(rollbackBtn, backBtn))

	return c.Edit(text, menu)
}

func (m *Module) handleRollbackSelect(c telebot.Context) error {
	_ = c.Respond(&telebot.CallbackResponse{})
	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) != 3 {
		return nil
	}
	clusterName, namespace, releaseName := parts[0], parts[1], parts[2]

	_, history, err := m.getReleaseDetail(clusterName, namespace, releaseName)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ %v", err))
	}

	menu := &telebot.ReplyMarkup{}
	rows := make([]telebot.Row, 0)
	for _, h := range history {
		if h.Version == 0 {
			continue // skip current
		}
		btn := menu.Data(
			fmt.Sprintf("Rev %d (%s)", h.Version, h.Chart.Metadata.AppVersion),
			"helm_rollback_confirm",
			m.sd(fmt.Sprintf("%s|%s|%s|%d", clusterName, namespace, releaseName, h.Version)),
		)
		rows = append(rows, menu.Row(btn))
	}
	menu.Inline(rows...)

	return c.Edit("Select revision to rollback to:", menu)
}

func (m *Module) handleRollbackConfirm(c telebot.Context) error {
	_ = c.Respond(&telebot.CallbackResponse{})
	parts := strings.SplitN(c.Callback().Data, "|", 4)
	if len(parts) != 4 {
		return nil
	}
	clusterName, namespace, releaseName := parts[0], parts[1], parts[2]
	var version int
	if _, err := fmt.Sscanf(parts[3], "%d", &version); err != nil {
		return c.Edit("❌ Invalid revision number")
	}

	ok, err := m.rbac.HasPermission(context.Background(), c.Sender().ID, rbac.PermHelmReleasesRollback)
	if err != nil || !ok {
		return c.Edit("🚫 You need `admin` permission to rollback Helm releases")
	}

	_ = c.Edit(fmt.Sprintf("🔄 Rolling back %s to Rev %d...", releaseName, version))

	if err := m.rollback(clusterName, namespace, releaseName, version); err != nil {
		return c.Edit(fmt.Sprintf("❌ Rollback failed: %v", err))
	}

	m.audit.Log(entity.AuditEntry{
		UserID:    c.Sender().ID,
		Username:  c.Sender().Username,
		Action:    "helm.releases.rollback",
		Resource:  releaseName,
		Cluster:   clusterName,
		Namespace: namespace,
		Status:    "success",
	})

	return c.Edit(fmt.Sprintf("✅ Rollback complete! %s is now at Rev %d", releaseName, version+1))
}

func (m *Module) handleRollbackCancel(c telebot.Context) error {
	_ = c.Respond(&telebot.CallbackResponse{Text: "Cancelled"})
	return c.Edit("❌ Rollback cancelled")
}

func (m *Module) handleRefresh(c telebot.Context) error {
	return m.handleNamespaceSelect(c)
}

// ─── Private data-access helpers (delegate to ReleaseClient) ─────────────────

// clientForCluster returns a ReleaseClient for the given cluster + namespace.
func (m *Module) clientForCluster(clusterName, namespace string) (ReleaseClient, error) {
	for _, cl := range m.clusters {
		if cl.Name == clusterName {
			return m.newClient(cl.Kubeconfig, namespace)
		}
	}
	return nil, fmt.Errorf("cluster %q not found", clusterName)
}

func (m *Module) listReleases(clusterName, namespace string) ([]*helmrelease.Release, error) {
	cl, err := m.clientForCluster(clusterName, namespace)
	if err != nil {
		return nil, err
	}
	return cl.ListReleases()
}

func (m *Module) getReleaseDetail(clusterName, namespace, releaseName string) (*helmrelease.Release, []*helmrelease.Release, error) {
	cl, err := m.clientForCluster(clusterName, namespace)
	if err != nil {
		return nil, nil, err
	}

	rel, err := cl.GetRelease(releaseName)
	if err != nil {
		return nil, nil, fmt.Errorf("get release: %w", err)
	}

	history, err := cl.GetHistory(releaseName)
	if err != nil {
		// History is best-effort; return release without it.
		return rel, nil, nil
	}
	return rel, history, nil
}

func (m *Module) rollback(clusterName, namespace, releaseName string, version int) error {
	cl, err := m.clientForCluster(clusterName, namespace)
	if err != nil {
		return err
	}
	return cl.Rollback(releaseName, version)
}

func statusEmoji(status helmrelease.Status) string {
	switch status {
	case helmrelease.StatusDeployed:
		return "✅"
	case helmrelease.StatusFailed:
		return "🔴"
	case helmrelease.StatusPendingInstall, helmrelease.StatusPendingUpgrade, helmrelease.StatusPendingRollback:
		return "🟡"
	default:
		return "⚪"
	}
}

func formatReleaseList(cluster, namespace string, releases []*helmrelease.Release) string {
	var sb strings.Builder
	ns := namespace
	if ns == "" {
		ns = "All Namespaces"
	}
	fmt.Fprintf(&sb, "⎈ Helm Releases — %s (cluster: %s)\n", ns, cluster)
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	if len(releases) == 0 {
		sb.WriteString("No releases found\n")
		return sb.String()
	}
	for _, r := range releases {
		status := string(r.Info.Status)
		age := time.Since(r.Info.LastDeployed.Time).Round(time.Minute)
		fmt.Fprintf(&sb, "%s %-20s %-10s %-10s Rev %-3d %s ago\n",
			statusEmoji(r.Info.Status),
			r.Name,
			r.Chart.Metadata.AppVersion,
			status,
			r.Version,
			age,
		)
	}
	return sb.String()
}

func formatReleaseDetail(rel *helmrelease.Release, history []*helmrelease.Release) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "⎈ %s (%s)\n", rel.Name, rel.Namespace)
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(&sb, "Chart:    %s\n", rel.Chart.Metadata.Name+"-"+rel.Chart.Metadata.Version)
	fmt.Fprintf(&sb, "App:      %s\n", rel.Chart.Metadata.AppVersion)
	fmt.Fprintf(&sb, "Status:   %s\n", rel.Info.Status)
	fmt.Fprintf(&sb, "Revision: %d\n", rel.Version)
	fmt.Fprintf(&sb, "Updated:  %s\n", rel.Info.LastDeployed.UTC().Format("2006-01-02 15:04 UTC"))

	if len(history) > 0 {
		sb.WriteString("\nHistory:\n")
		for _, h := range history {
			marker := ""
			if h.Version == rel.Version {
				marker = " (current)"
			}
			fmt.Fprintf(&sb, "  Rev %-3d — %s%s\n", h.Version, h.Chart.Metadata.AppVersion, marker)
		}
	}
	return sb.String()
}
