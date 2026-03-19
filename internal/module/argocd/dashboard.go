package argocd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/rbac"
	pkgargocd "github.com/d9042n/telekube/pkg/argocd"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// handleDashboard handles the /dashboard command — aggregate view across all instances.
func (m *Module) handleDashboard(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermArgoCDAppsList)
	if !allowed {
		return c.Send("⛔ You don't have permission to view the ArgoCD dashboard.")
	}

	return m.sendDashboard(c)
}

// handleDashboardRefresh refreshes the dashboard.
func (m *Module) handleDashboardRefresh(c telebot.Context) error {
	return m.sendDashboard(c)
}

// handleDashboardOutOfSync shows all OutOfSync apps.
func (m *Module) handleDashboardOutOfSync(c telebot.Context) error {
	return m.sendFilteredApps(c, "OutOfSync")
}

// handleDashboardDegraded shows all Degraded apps.
func (m *Module) handleDashboardDegraded(c telebot.Context) error {
	return m.sendFilteredApps(c, "Degraded")
}

// sendDashboard aggregates status across all ArgoCD instances.
func (m *Module) sendDashboard(c telebot.Context) error {
	if len(m.instances) == 0 {
		return sendOrEdit(c, "⚠️ No ArgoCD instances configured.", nil)
	}

	type instanceSummary struct {
		name       string
		clusters   []string
		total      int
		healthy    int
		outOfSync  int
		degraded   int
		outApps    []pkgargocd.Application
		degradedApps []pkgargocd.Application
	}

	var summaries []instanceSummary
	var totalHealthy, totalOutOfSync, totalDegraded int

	for _, inst := range m.instances {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		apps, err := inst.client.ListApplications(ctx, pkgargocd.ListOpts{})
		cancel()

		if err != nil {
			m.logger.Warn("failed to list apps for dashboard",
				zap.String("instance", inst.cfg.Name),
				zap.Error(err),
			)
			continue
		}

		sum := instanceSummary{
			name:     inst.cfg.Name,
			clusters: inst.cfg.Clusters,
			total:    len(apps),
		}
		for _, app := range apps {
			switch {
			case app.SyncStatus == "OutOfSync":
				sum.outOfSync++
				sum.outApps = append(sum.outApps, app)
			case app.HealthStatus == "Degraded" || app.HealthStatus == "Missing":
				sum.degraded++
				sum.degradedApps = append(sum.degradedApps, app)
			default:
				sum.healthy++
			}
		}
		summaries = append(summaries, sum)
		totalHealthy += sum.healthy
		totalOutOfSync += sum.outOfSync
		totalDegraded += sum.degraded
	}

	var sb strings.Builder
	sb.WriteString("📊 *GitOps Dashboard*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	for _, sum := range summaries {
		clusters := strings.Join(sum.clusters, ", ")
		if clusters == "" {
			clusters = "n/a"
		}
		sb.WriteString(fmt.Sprintf("📍 *%s* (%s)\n", sum.name, clusters))
		if sum.healthy > 0 {
			sb.WriteString(fmt.Sprintf("   ✅ %d Synced & Healthy\n", sum.healthy))
		}
		if sum.outOfSync > 0 {
			appNames := make([]string, 0, len(sum.outApps))
			for _, a := range sum.outApps {
				appNames = append(appNames, a.Name)
			}
			sb.WriteString(fmt.Sprintf("   🟡 %d OutOfSync: %s\n", sum.outOfSync, strings.Join(appNames, ", ")))
		}
		if sum.degraded > 0 {
			appNames := make([]string, 0, len(sum.degradedApps))
			for _, a := range sum.degradedApps {
				appNames = append(appNames, a.Name)
			}
			sb.WriteString(fmt.Sprintf("   🔴 %d Degraded: %s\n", sum.degraded, strings.Join(appNames, ", ")))
		}
		sb.WriteString(fmt.Sprintf("   Total: %d apps\n\n", sum.total))
	}

	sb.WriteString("═══════════════════════════\n")
	sb.WriteString(fmt.Sprintf("Overall: %d ✅ | %d 🟡 | %d 🔴\n",
		totalHealthy, totalOutOfSync, totalDegraded))

	menu := &telebot.ReplyMarkup{}
	btnOutOfSync := menu.Data("🔍 Show OutOfSync", "argo_dash_outofsync", "")
	btnDegraded := menu.Data("🔍 Show Degraded", "argo_dash_degraded", "")
	btnRefresh := menu.Data("🔄 Refresh", "argo_dash_refresh", "")
	menu.Inline(
		menu.Row(btnOutOfSync, btnDegraded),
		menu.Row(btnRefresh),
	)

	return sendOrEdit(c, sb.String(), menu)
}

// sendFilteredApps shows apps by filter ("OutOfSync" or "Degraded").
func (m *Module) sendFilteredApps(c telebot.Context, filterStatus string) error {
	type filteredApp struct {
		instanceName string
		clusters     []string
		app          pkgargocd.Application
	}
	var filtered []filteredApp

	for _, inst := range m.instances {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		apps, err := inst.client.ListApplications(ctx, pkgargocd.ListOpts{})
		cancel()
		if err != nil {
			continue
		}
		for _, app := range apps {
			shouldInclude := false
			switch filterStatus {
			case "OutOfSync":
				shouldInclude = app.SyncStatus == "OutOfSync"
			case "Degraded":
				shouldInclude = app.HealthStatus == "Degraded" || app.HealthStatus == "Missing"
			}
			if shouldInclude {
				filtered = append(filtered, filteredApp{
					instanceName: inst.cfg.Name,
					clusters:     inst.cfg.Clusters,
					app:          app,
				})
			}
		}
	}

	emoji := "🟡"
	if filterStatus == "Degraded" {
		emoji = "🔴"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s *%s Applications*\n", emoji, filterStatus))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	if len(filtered) == 0 {
		sb.WriteString(fmt.Sprintf("✅ No %s applications!\n", filterStatus))
	} else {
		for _, fa := range filtered {
			app := fa.app
			sb.WriteString(fmt.Sprintf("📦 *%s* (%s)\n", app.Name, fa.instanceName))
			if len(fa.clusters) > 0 {
				sb.WriteString(fmt.Sprintf("   Clusters: %s\n", strings.Join(fa.clusters, ", ")))
			}
			sb.WriteString("\n")

			data := fmt.Sprintf("%s|%s", fa.instanceName, app.Name)
			btnSync := menu.Data("⚡ Sync "+app.Name, "argo_sync", data)
			rows = append(rows, menu.Row(btnSync))
		}
	}

	btnBack := menu.Data("◀️ Dashboard", "argo_dash_refresh", "")
	rows = append(rows, menu.Row(btnBack))
	menu.Inline(rows...)

	return sendOrEdit(c, sb.String(), menu)
}
