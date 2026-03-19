package argocd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/oklog/ulid/v2"
	"gopkg.in/telebot.v3"
	"go.uber.org/zap"
)

// handleFreeze handles the /freeze command.
func (m *Module) handleFreeze(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermArgoCDFreezeManage)
	if !allowed {
		return c.Send("⛔ You don't have permission to manage deployment freezes.")
	}

	return m.sendFreezeStatus(c, ctx)
}

// handleFreezeCreate handles the [❄️ Create Freeze] button.
func (m *Module) handleFreezeCreate(c telebot.Context) error {
	_ = c.Respond(&telebot.CallbackResponse{})

	// Show scope options
	menu := &telebot.ReplyMarkup{}
	rows := []telebot.Row{
		menu.Row(menu.Data("🌍 All Clusters", "argo_freeze_scope", "all")),
	}
	for _, inst := range m.instances {
		for _, cluster := range inst.cfg.Clusters {
			btn := menu.Data("📍 "+cluster, "argo_freeze_scope", cluster)
			rows = append(rows, menu.Row(btn))
		}
	}
	rows = append(rows, menu.Row(menu.Data("❌ Cancel", "argo_noop", "")))
	menu.Inline(rows...)

	text := "❄️ *Create Deployment Freeze*\n\nSelect scope:"
	_, err := c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// handleFreezeScope handles scope selection and shows duration options.
func (m *Module) handleFreezeScope(c telebot.Context) error {
	scope := c.Callback().Data
	_ = c.Respond(&telebot.CallbackResponse{})

	menu := &telebot.ReplyMarkup{}
	durations := [][2]string{
		{"30 minutes", "30m"},
		{"1 hour", "1h"},
		{"2 hours", "2h"},
		{"4 hours", "4h"},
		{"8 hours", "8h"},
		{"24 hours", "24h"},
	}
	var rows []telebot.Row
	for _, d := range durations {
		data := fmt.Sprintf("%s|%s", scope, d[1])
		btn := menu.Data("⏱ "+d[0], "argo_freeze_duration", data)
		rows = append(rows, menu.Row(btn))
	}
	rows = append(rows, menu.Row(menu.Data("❌ Cancel", "argo_noop", "")))
	menu.Inline(rows...)

	scopeLabel := scope
	if scope == "all" {
		scopeLabel = "All Clusters"
	}

	text := fmt.Sprintf("❄️ *Create Deployment Freeze*\nScope: %s\n\nSelect freeze duration:", scopeLabel)
	_, err := c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// handleFreezeDuration shows a confirmation dialog for the selected scope and duration.
func (m *Module) handleFreezeDuration(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}
	scope, duration := parts[0], parts[1]

	d, err := time.ParseDuration(duration)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid duration"})
	}
	expiresAt := time.Now().UTC().Add(d)

	scopeLabel := scope
	if scope == "all" {
		scopeLabel = "🌍 All Clusters"
	} else {
		scopeLabel = "📍 " + scope
	}

	text := fmt.Sprintf("⚠️ *Confirm Deployment Freeze*\n━━━━━━━━━━━━━━━━━━\n"+
		"Scope:    %s\n"+
		"Duration: %s\n"+
		"Expires:  %s\n\n"+
		"⚠️ This will block all sync and rollback operations!\n",
		scopeLabel, duration, expiresAt.Format("2006-01-02 15:04 UTC"))

	menu := &telebot.ReplyMarkup{}
	confirmData := fmt.Sprintf("%s|%s", scope, duration)
	btnConfirm := menu.Data("❄️ Freeze Now", "argo_freeze_confirm", confirmData)
	btnCancel := menu.Data("❌ Cancel", "argo_noop", "")
	menu.Inline(menu.Row(btnConfirm, btnCancel))

	_ = c.Respond(&telebot.CallbackResponse{})
	_, err = c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// handleFreezeConfirm executes the freeze creation.
func (m *Module) handleFreezeConfirm(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}
	scope, duration := parts[0], parts[1]

	d, err := time.ParseDuration(duration)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid duration"})
	}

	now := time.Now().UTC()
	freeze := &entity.DeploymentFreeze{
		ID:        ulid.Make().String(),
		Scope:     scope,
		Reason:    "",
		CreatedBy: user.TelegramID,
		CreatedAt: now,
		ExpiresAt: now.Add(d),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.freeze.Create(ctx, freeze); err != nil {
		m.logger.Error("failed to create freeze", zap.Error(err))
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Failed to create freeze"})
	}

	// Audit
	m.audit.Log(entity.AuditEntry{
		ID:        ulid.Make().String(),
		UserID:    user.TelegramID,
		Username:  user.Username,
		Action:    "argocd.freeze.create",
		Resource:  fmt.Sprintf("freeze/%s", freeze.ID),
		Cluster:   scope,
		Status:    entity.AuditStatusSuccess,
		Details:   map[string]interface{}{"scope": scope, "duration": duration, "expires_at": freeze.ExpiresAt},
		OccurredAt: now,
	})

	scopeLabel := scope
	if scope == "all" {
		scopeLabel = "All Clusters"
	}

	text := fmt.Sprintf("❄️ *Deployment Freeze Active*\n\n"+
		"Scope:    %s\n"+
		"Duration: %s\n"+
		"Expires:  %s\n\n"+
		"🚫 All sync and rollback operations are now BLOCKED.\n"+
		"Use /freeze to manage or thaw.",
		scopeLabel, duration, freeze.ExpiresAt.Format("2006-01-02 15:04 UTC"))

	menu := &telebot.ReplyMarkup{}
	thawBtn := menu.Data("☀️ Thaw Now", "argo_freeze_thaw", freeze.ID)
	menu.Inline(menu.Row(thawBtn))

	_ = c.Respond(&telebot.CallbackResponse{Text: "❄️ Freeze created!"})
	_, editErr := c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return editErr
}

// handleFreezeThaw handles early thawing of a freeze.
func (m *Module) handleFreezeThaw(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	freezeID := c.Callback().Data

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.freeze.Thaw(ctx, freezeID, user.TelegramID); err != nil {
		m.logger.Error("failed to thaw freeze", zap.String("id", freezeID), zap.Error(err))
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Failed to thaw freeze"})
	}

	m.audit.Log(entity.AuditEntry{
		ID:        ulid.Make().String(),
		UserID:    user.TelegramID,
		Username:  user.Username,
		Action:    "argocd.freeze.thaw",
		Resource:  fmt.Sprintf("freeze/%s", freezeID),
		Status:    entity.AuditStatusSuccess,
		Details:   map[string]interface{}{"freeze_id": freezeID},
		OccurredAt: time.Now().UTC(),
	})

	_ = c.Respond(&telebot.CallbackResponse{Text: "☀️ Freeze removed!"})
	return sendOrEdit(c, "☀️ *Deployment Freeze Removed*\n\nSync and rollback operations are now unblocked.", nil)
}

// handleFreezeHistory displays recent freeze history.
func (m *Module) handleFreezeHistory(c telebot.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	freezes, err := m.freeze.List(ctx, 10)
	if err != nil {
		return sendOrEdit(c, "⚠️ Failed to load freeze history.", nil)
	}

	var sb strings.Builder
	sb.WriteString("📋 *Deployment Freeze History*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if len(freezes) == 0 {
		sb.WriteString("No freeze history found.\n")
	}

	for _, f := range freezes {
		scope := f.Scope
		if scope == "all" {
			scope = "All Clusters"
		}
		statusEmoji := "❄️"
		statusLabel := "Active"
		if !f.IsActive() {
			if f.ThawedAt != nil {
				statusEmoji = "☀️"
				statusLabel = "Thawed"
			} else {
				statusEmoji = "✅"
				statusLabel = "Expired"
			}
		}
		sb.WriteString(fmt.Sprintf("%s *%s* — %s\n", statusEmoji, scope, statusLabel))
		sb.WriteString(fmt.Sprintf("  Created: %s\n", f.CreatedAt.UTC().Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("  Expires: %s\n", f.ExpiresAt.UTC().Format("2006-01-02 15:04")))
		if f.IsActive() {
			sb.WriteString(fmt.Sprintf("  ⏱ Remaining: %s\n", f.RemainingDuration().Truncate(time.Minute)))
		}
		sb.WriteString("\n")
	}

	menu := &telebot.ReplyMarkup{}
	btnNew := menu.Data("❄️ New Freeze", "argo_freeze_create", "")
	menu.Inline(menu.Row(btnNew))

	_ = c.Respond(&telebot.CallbackResponse{})
	return sendOrEdit(c, sb.String(), menu)
}

// sendFreezeStatus shows the current freeze status.
func (m *Module) sendFreezeStatus(c telebot.Context, ctx context.Context) error {
	freeze, err := m.freeze.GetActive(ctx)
	if err != nil {
		return c.Send("⚠️ Failed to check freeze status.")
	}

	menu := &telebot.ReplyMarkup{}
	var text string

	if freeze != nil && freeze.IsActive() {
		scope := freeze.Scope
		if scope == "all" {
			scope = "All Clusters"
		}
		text = fmt.Sprintf("❄️ *Deployment Freeze ACTIVE*\n\n"+
			"Scope:     %s\n"+
			"Remaining: %s\n"+
			"Expires:   %s\n\n"+
			"🚫 Sync and rollback operations are BLOCKED.",
			scope,
			freeze.RemainingDuration().Truncate(time.Minute),
			freeze.ExpiresAt.Format("2006-01-02 15:04 UTC"))

		thawBtn := menu.Data("☀️ Thaw Now", "argo_freeze_thaw", freeze.ID)
		histBtn := menu.Data("📋 History", "argo_freeze_history", "")
		menu.Inline(menu.Row(thawBtn, histBtn))
	} else {
		text = "☀️ *Deployment Status: Normal*\n\nNo active freeze. All operations are allowed.\n"
		createBtn := menu.Data("❄️ Create Freeze", "argo_freeze_create", "")
		histBtn := menu.Data("📋 History", "argo_freeze_history", "")
		menu.Inline(menu.Row(createBtn, histBtn))
	}

	return sendOrEdit(c, text, menu)
}

// formatFreezeBlocked formats the message shown when an operation is blocked by a freeze.
func formatFreezeBlocked(freeze *entity.DeploymentFreeze) string {
	scope := freeze.Scope
	if scope == "all" {
		scope = "all clusters"
	}
	return fmt.Sprintf("❄️ *Deployment Freeze Active*\n\n"+
		"🚫 This operation is BLOCKED by an active deployment freeze.\n\n"+
		"Scope:     %s\n"+
		"Remaining: %s\n"+
		"Expires:   %s\n\n"+
		"Contact an admin to thaw the freeze early.",
		scope,
		freeze.RemainingDuration().Truncate(time.Minute),
		freeze.ExpiresAt.Format("2006-01-02 15:04 UTC"))
}
