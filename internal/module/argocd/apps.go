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

// handleApps handles the /apps command — list ArgoCD applications.
func (m *Module) handleApps(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermArgoCDAppsList)
	if !allowed {
		return c.Send("⛔ You don't have permission to list ArgoCD applications.")
	}

	if len(m.instances) == 0 {
		return c.Send("⚠️ No ArgoCD instances configured.")
	}

	// If multiple instances, show instance selector first
	if len(m.instances) > 1 {
		return m.sendInstanceSelector(c)
	}

	// Single instance — go straight to apps
	return m.sendAppList(c, m.instances[0].cfg.Name)
}

// handleInstanceSelect handles selection when multiple ArgoCD instances exist.
func (m *Module) handleInstanceSelect(c telebot.Context) error {
	return m.sendAppList(c, c.Callback().Data)
}

// handleAppsRefresh refreshes the app list.
func (m *Module) handleAppsRefresh(c telebot.Context) error {
	return m.sendAppList(c, c.Callback().Data)
}

// handleAppsBack returns to instance selector or re-sends app list.
func (m *Module) handleAppsBack(c telebot.Context) error {
	if len(m.instances) > 1 {
		return m.sendInstanceSelector(c)
	}
	if len(m.instances) > 0 {
		return m.sendAppList(c, m.instances[0].cfg.Name)
	}
	return sendOrEdit(c, "⚠️ No ArgoCD instances configured.", nil)
}

// sendInstanceSelector shows inline buttons to select an ArgoCD instance.
func (m *Module) sendInstanceSelector(c telebot.Context) error {
	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row
	for _, inst := range m.instances {
		btn := menu.Data(fmt.Sprintf("📍 %s", inst.cfg.Name), "argo_inst_select", inst.cfg.Name)
		rows = append(rows, menu.Row(btn))
	}
	menu.Inline(rows...)
	text := "🚀 *ArgoCD Instances*\n\nSelect an instance to view applications:"
	return sendOrEdit(c, text, menu)
}

// sendAppList fetches and displays applications for a given instance.
func (m *Module) sendAppList(c telebot.Context, instanceName string) error {
	inst, err := m.getInstanceByName(instanceName)
	if err != nil {
		return sendOrEdit(c, "⚠️ ArgoCD instance not found.", nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	apps, err := inst.client.ListApplications(ctx, pkgargocd.ListOpts{})
	if err != nil {
		m.logger.Error("failed to list argocd applications",
			zap.String("instance", instanceName),
			zap.Error(err),
		)
		return sendOrEdit(c, "⚠️ Failed to list ArgoCD applications. Is ArgoCD reachable?", nil)
	}

	if len(apps) == 0 {
		return sendOrEdit(c, fmt.Sprintf("🚀 No applications found in *%s*.", instanceName), nil)
	}

	// Count statuses for summary
	var synced, outOfSync, degraded int
	for _, app := range apps {
		switch {
		case app.SyncStatus == "OutOfSync":
			outOfSync++
		case app.HealthStatus == "Degraded" || app.HealthStatus == "Missing":
			degraded++
		default:
			synced++
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🚀 *ArgoCD Applications* (%s)\n", instanceName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	for _, app := range apps {
		emoji := syncStatusEmoji(app.SyncStatus, app.HealthStatus)
		rev := shortRev(app.CurrentRev)
		sb.WriteString(fmt.Sprintf("%s `%-20s` %-10s %-10s %s\n",
			emoji, app.Name, app.SyncStatus, app.HealthStatus, rev))
	}
	sb.WriteString(fmt.Sprintf("\nSummary: %d ✅ Healthy, %d 🟡 OutOfSync, %d 🔴 Degraded\n",
		synced, outOfSync, degraded))

	// Build keyboard: one button per app + refresh
	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row
	for _, app := range apps {
		emoji := syncStatusEmoji(app.SyncStatus, app.HealthStatus)
		data := fmt.Sprintf("%s|%s", instanceName, app.Name)
		btn := menu.Data(fmt.Sprintf("%s %s", emoji, app.Name), "argo_app_detail", data)
		rows = append(rows, menu.Row(btn))
	}
	rows = append(rows, menu.Row(
		menu.Data("🔄 Refresh", "argo_apps_refresh", instanceName),
		menu.Data("📊 Dashboard", "argo_dash_refresh", ""),
	))
	menu.Inline(rows...)

	return sendOrEdit(c, sb.String(), menu)
}

// handleAppDetail shows detailed status of a specific ArgoCD application.
func (m *Module) handleAppDetail(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}
	instanceName, appName := parts[0], parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermArgoCDAppsView)
	if !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "⛔ No permission"})
	}

	inst, err := m.getInstanceByName(instanceName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Instance not found"})
	}

	appStatus, err := inst.client.GetApplicationStatus(ctx, appName)
	if err != nil {
		m.logger.Error("failed to get argocd app status",
			zap.String("instance", instanceName),
			zap.String("app", appName),
			zap.Error(err),
		)
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Failed to get application status"})
	}

	text := formatAppStatusDetail(appStatus, instanceName)
	menu := buildAppDetailKeyboard(instanceName, appName)

	_, err = c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// formatAppStatusDetail formats the detailed view of an application status.
func formatAppStatusDetail(app *pkgargocd.ApplicationStatus, instanceName string) string {
	var sb strings.Builder

	syncEmoji := syncStatusEmoji(app.SyncStatus, app.HealthStatus)
	sb.WriteString(fmt.Sprintf("🚀 *%s*\n", app.Name))
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("Sync Status:  %s %s\n", syncStatusEmoji(app.SyncStatus, "Healthy"), app.SyncStatus))
	sb.WriteString(fmt.Sprintf("Health:       %s %s\n", syncEmoji, app.HealthStatus))
	sb.WriteString(fmt.Sprintf("Project:      %s\n", app.Project))
	sb.WriteString(fmt.Sprintf("Instance:     %s\n", instanceName))
	if app.RepoURL != "" {
		sb.WriteString(fmt.Sprintf("Repo:         %s\n", app.RepoURL))
	}
	if app.Path != "" {
		sb.WriteString(fmt.Sprintf("Path:         %s\n", app.Path))
	}
	if app.TargetRev != "" {
		sb.WriteString(fmt.Sprintf("Target Rev:   %s\n", app.TargetRev))
	}
	if app.CurrentRev != "" {
		sb.WriteString(fmt.Sprintf("Current Rev:  `%s`\n", shortRev(app.CurrentRev)))
	}

	if len(app.Resources) > 0 {
		sb.WriteString("\n*Resources:*\n")
		shown := 0
		for _, r := range app.Resources {
			if shown >= 10 {
				sb.WriteString(fmt.Sprintf("  ...and %d more\n", len(app.Resources)-shown))
				break
			}
			emoji := resourceStatusEmoji(r.Status, r.Health)
			sb.WriteString(fmt.Sprintf("  %s %s/%s\n", emoji, r.Kind, r.Name))
			shown++
		}
	}

	if app.LastSyncAt != nil {
		ago := time.Since(*app.LastSyncAt).Truncate(time.Minute)
		sb.WriteString(fmt.Sprintf("\nLast Sync: %s (%s ago)\n",
			app.LastSyncAt.UTC().Format("2006-01-02 15:04:05"),
			ago))
	}
	if app.LastSyncBy != "" {
		sb.WriteString(fmt.Sprintf("Deployed by: %s\n", app.LastSyncBy))
	}

	return sb.String()
}

// buildAppDetailKeyboard builds the action keyboard for an app detail view.
func buildAppDetailKeyboard(instanceName, appName string) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}
	data := fmt.Sprintf("%s|%s", instanceName, appName)

	btnSync := menu.Data("⚡ Sync", "argo_sync", data)
	btnDiff := menu.Data("🔍 Diff", "argo_diff", data)
	btnRollback := menu.Data("⏪ Rollback", "argo_rollback", data)
	btnBack := menu.Data("◀️ Back", "argo_apps_back", instanceName)

	menu.Inline(
		menu.Row(btnSync, btnDiff, btnRollback),
		menu.Row(btnBack),
	)
	return menu
}

// sendOrEdit sends or edits a message depending on whether it's a callback.
func sendOrEdit(c telebot.Context, text string, menu *telebot.ReplyMarkup) error {
	opts := []interface{}{telebot.ModeMarkdown}
	if menu != nil {
		opts = append(opts, menu)
	}
	if c.Callback() != nil {
		_, err := c.Bot().Edit(c.Callback().Message, text, opts...)
		return err
	}
	return c.Send(text, opts...)
}
