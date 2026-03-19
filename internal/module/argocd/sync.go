package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module/approval"
	"github.com/d9042n/telekube/internal/rbac"
	pkgargocd "github.com/d9042n/telekube/pkg/argocd"
	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// handleSync handles the [⚡ Sync] button — shows diff then sync options.
func (m *Module) handleSync(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}
	instanceName, appName := parts[0], parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermArgoCDAppsSync)
	if !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "⛔ You don't have permission to sync applications"})
	}

	// Check deployment freeze
	freeze, err := m.freeze.GetActiveForCluster(ctx, instanceName)
	if err == nil && freeze != nil && freeze.IsActive() {
		return sendOrEdit(c, formatFreezeBlocked(freeze), nil)
	}

	_ = c.Respond(&telebot.CallbackResponse{Text: "🔄 Fetching diff..."})

	// Fetch diff
	diffCtx, diffCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer diffCancel()

	inst, err := m.getInstanceByName(instanceName)
	if err != nil {
		return sendOrEdit(c, "⚠️ ArgoCD instance not found.", nil)
	}

	diff, err := inst.client.GetApplicationDiff(diffCtx, appName)
	if err != nil {
		m.logger.Warn("failed to get diff, proceeding without",
			zap.String("app", appName),
			zap.Error(err),
		)
		diff = &pkgargocd.DiffResult{AppName: appName}
	}

	text := formatDiffPreview(diff)
	menu := buildSyncOptionsKeyboard(instanceName, appName, "")

	_, editErr := c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return editErr
}

// handleSyncNow initiates a normal sync (no prune).
func (m *Module) handleSyncNow(c telebot.Context) error {
	return m.initiateSyncConfirm(c, "sync_now")
}

// handleSyncPrune initiates a sync with prune.
func (m *Module) handleSyncPrune(c telebot.Context) error {
	return m.initiateSyncConfirm(c, "sync_prune")
}

// handleSyncForce initiates a forced sync.
func (m *Module) handleSyncForce(c telebot.Context) error {
	return m.initiateSyncConfirm(c, "sync_force")
}

// handleSyncCancel cancels the sync and returns to app detail.
func (m *Module) handleSyncCancel(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) < 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}
	return m.handleAppDetail(c)
}

// initiateSyncConfirm shows a confirmation dialog for the selected sync mode.
func (m *Module) initiateSyncConfirm(c telebot.Context, mode string) error {
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}
	instanceName, appName := parts[0], parts[1]

	modeLabel := map[string]string{
		"sync_now":   "Normal sync",
		"sync_prune": "Sync with prune (deletes removed resources)",
		"sync_force": "Force sync (replaces resources)",
	}[mode]

	text := fmt.Sprintf("⚠️ *Confirm Sync: %s*\n━━━━━━━━━━━━━━━━━━\nApp:    `%s`\nMode:   %s\n\nProceed?",
		appName, appName, modeLabel)

	confirmData := fmt.Sprintf("%s|%s|%s", instanceName, appName, mode)
	menu := &telebot.ReplyMarkup{}
	btnConfirm := menu.Data("✅ Confirm", "argo_sync_confirm", confirmData)
	btnCancel := menu.Data("❌ Cancel", "argo_sync_cancel", confirmData)
	menu.Inline(menu.Row(btnConfirm, btnCancel))

	_, err := c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// handleSyncConfirm executes the sync after confirmation.
func (m *Module) handleSyncConfirm(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) != 3 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}
	instanceName, appName, mode := parts[0], parts[1], parts[2]

	// Check approval requirement
	if m.approval != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		needsApproval, err := m.approval.CheckAndSubmit(ctx, c, approval.ApprovalInput{
			UserID:       user.TelegramID,
			Username:     user.Username,
			Action:       "argocd.apps.sync",
			Resource:     appName,
			Cluster:      instanceName,
			Details:      map[string]interface{}{"mode": mode, "instance": instanceName},
			CallbackData: c.Callback().Data,
		})
		if err != nil {
			m.logger.Error("approval check failed", zap.Error(err))
			// Fall through to execute directly if approval check itself fails
		}
		if needsApproval {
			return sendOrEdit(c, fmt.Sprintf("📋 Sync for `%s` requires approval.\nWaiting for admin approval...", appName), nil)
		}
	}

	opts := pkgargocd.SyncOpts{}
	switch mode {
	case "sync_prune":
		opts.Prune = true
	case "sync_force":
		opts.Force = true
	}

	_ = c.Respond(&telebot.CallbackResponse{Text: "🔄 Syncing..."})

	syncCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inst, err := m.getInstanceByName(instanceName)
	if err != nil {
		return sendOrEdit(c, "⚠️ ArgoCD instance not found.", nil)
	}

	result, err := inst.client.SyncApplication(syncCtx, appName, opts)

	// Audit
	status := entity.AuditStatusSuccess
	errMsg := ""
	if err != nil {
		status = entity.AuditStatusError
		errMsg = err.Error()
	}
	m.audit.Log(entity.AuditEntry{
		ID:        ulid.Make().String(),
		UserID:    user.TelegramID,
		Username:  user.Username,
		Action:    "argocd.app.sync",
		Resource:  fmt.Sprintf("app/%s", appName),
		Cluster:   instanceName,
		Status:    status,
		Error:     errMsg,
		Details:   map[string]interface{}{"mode": mode, "instance": instanceName},
		OccurredAt: time.Now().UTC(),
	})

	if err != nil {
		m.logger.Error("sync failed",
			zap.String("app", appName),
			zap.String("instance", instanceName),
			zap.Error(err),
		)
		return sendOrEdit(c, fmt.Sprintf("❌ Sync failed for `%s`:\n%s", appName, err.Error()), nil)
	}

	text := formatSyncResult(result, appName, user.Username)
	btnBack := &telebot.ReplyMarkup{}
	backData := fmt.Sprintf("%s|%s", instanceName, appName)
	btn := btnBack.Data("◀️ Back to App", "argo_app_detail", backData)
	btnBack.Inline(btnBack.Row(btn))

	_, editErr := c.Bot().Edit(c.Callback().Message, text, btnBack, telebot.ModeMarkdown)
	return editErr
}

// formatDiffPreview formats the diff preview before sync.
func formatDiffPreview(diff *pkgargocd.DiffResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 *Pending Changes for* `%s`\n", diff.AppName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if diff.Changed == 0 && len(diff.Resources) == 0 {
		sb.WriteString("✅ No pending changes detected.\n")
	} else {
		for _, r := range diff.Resources {
			if r.Diff == "" {
				continue
			}
			sb.WriteString(fmt.Sprintf("📦 *%s/%s*:\n", r.Kind, r.Name))
			diffLines := formatResourceDiff(r)
			if len(diffLines) > 0 {
				sb.WriteString(diffLines)
			}
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("Summary: %d changed, %d added, %d removed\n",
			diff.Changed, diff.Added, diff.Removed))
	}
	sb.WriteString("\n*Sync Options:*")
	return sb.String()
}

// formatResourceDiff formats a single resource diff, redacting secrets.
func formatResourceDiff(r pkgargocd.ResourceDiff) string {
	if r.Kind == "Secret" {
		return "  ⚠️ `<secret value redacted>`\n"
	}
	// Try to extract meaningful diff info from JSON comparison
	if r.Live != "" && r.Target != "" {
		var live, target map[string]interface{}
		if json.Unmarshal([]byte(r.Live), &live) == nil &&
			json.Unmarshal([]byte(r.Target), &target) == nil {
			return computeSimpleDiff(live, target)
		}
	}
	return fmt.Sprintf("  %s\n", r.Diff)
}

// computeSimpleDiff computes a simple key-level diff between two maps.
func computeSimpleDiff(live, target map[string]interface{}) string {
	var sb strings.Builder
	const maxLines = 5
	lines := 0

	for k, tv := range target {
		if lines >= maxLines {
			sb.WriteString("  ...\n")
			break
		}
		lv, exists := live[k]
		if !exists {
			sb.WriteString(fmt.Sprintf("  + %s: %v\n", k, tv))
			lines++
		} else if fmt.Sprintf("%v", lv) != fmt.Sprintf("%v", tv) {
			sb.WriteString(fmt.Sprintf("  ~ %s: %v → %v\n", k, lv, tv))
			lines++
		}
	}
	for k := range live {
		if _, ok := target[k]; !ok && lines < maxLines {
			sb.WriteString(fmt.Sprintf("  - %s\n", k))
			lines++
		}
	}
	return sb.String()
}

// buildSyncOptionsKeyboard builds the keyboard for choosing sync mode.
func buildSyncOptionsKeyboard(instanceName, appName, _ string) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}
	data := fmt.Sprintf("%s|%s", instanceName, appName)

	btnNow := menu.Data("⚡ Sync Now", "argo_sync_now", data)
	btnPrune := menu.Data("⚡ Sync (Prune)", "argo_sync_prune", data)
	btnForce := menu.Data("⚡ Sync (Force)", "argo_sync_force", data)
	btnCancel := menu.Data("❌ Cancel", "argo_sync_cancel", data)

	menu.Inline(
		menu.Row(btnNow, btnPrune),
		menu.Row(btnForce),
		menu.Row(btnCancel),
	)
	return menu
}

// formatSyncResult formats the sync result message.
func formatSyncResult(result *pkgargocd.SyncResult, appName, triggeredBy string) string {
	var sb strings.Builder
	phaseEmoji := "✅"
	if result.Phase != "" && result.Phase != "Succeeded" && result.Phase != "Running" {
		phaseEmoji = "❌"
	}

	sb.WriteString(fmt.Sprintf("%s *Sync Complete: %s*\n\n", phaseEmoji, appName))
	if result.Phase != "" {
		sb.WriteString(fmt.Sprintf("Status:   %s\n", result.Phase))
	}
	if result.Revision != "" {
		sb.WriteString(fmt.Sprintf("Revision: `%s`\n", shortRev(result.Revision)))
	}
	if result.Message != "" {
		sb.WriteString(fmt.Sprintf("Message:  %s\n", result.Message))
	}

	if len(result.Results) > 0 {
		sb.WriteString("\n*Resources:*\n")
		for _, r := range result.Results {
			emoji := "✅"
			if r.Status != "Synced" {
				emoji = "❌"
			}
			sb.WriteString(fmt.Sprintf("  %s %s/%s\n", emoji, r.Kind, r.Name))
		}
	}

	sb.WriteString(fmt.Sprintf("\nTriggered by: @%s at %s",
		triggeredBy, time.Now().UTC().Format("2006-01-02 15:04:05")))
	return sb.String()
}
