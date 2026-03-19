package argocd

import (
	"context"
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

const maxHistoryItems = 10

// handleRollback handles the [⏪ Rollback] button — shows deployment history.
func (m *Module) handleRollback(c telebot.Context) error {
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

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermArgoCDAppsRollback)
	if !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "⛔ You don't have permission to rollback applications"})
	}

	// Check freeze
	freeze, err := m.freeze.GetActiveForCluster(ctx, instanceName)
	if err == nil && freeze != nil && freeze.IsActive() {
		return sendOrEdit(c, formatFreezeBlocked(freeze), nil)
	}

	histCtx, histCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer histCancel()

	inst, err := m.getInstanceByName(instanceName)
	if err != nil {
		return sendOrEdit(c, "⚠️ ArgoCD instance not found.", nil)
	}

	history, err := inst.client.GetApplicationHistory(histCtx, appName)
	if err != nil {
		m.logger.Error("failed to get app history",
			zap.String("app", appName),
			zap.Error(err),
		)
		return sendOrEdit(c, "⚠️ Failed to get deployment history.", nil)
	}

	if len(history) == 0 {
		return sendOrEdit(c, fmt.Sprintf("📋 No deployment history for `%s`.", appName), nil)
	}

	text, menu := formatRollbackHistory(history, instanceName, appName)
	_, editErr := c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return editErr
}

// handleRollbackSelect shows confirmation for a selected revision.
func (m *Module) handleRollbackSelect(c telebot.Context) error {
	// data: instanceName|appName|revisionID|revisionHash
	parts := strings.SplitN(c.Callback().Data, "|", 4)
	if len(parts) != 4 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}
	instanceName, appName, revIDStr, revHash := parts[0], parts[1], parts[2], parts[3]

	text := fmt.Sprintf("⚠️ *EMERGENCY ROLLBACK*\n━━━━━━━━━━━━━━━━━━\n"+
		"App:     `%s`\n"+
		"To:      Rev %s (`%s`)\n"+
		"Instance: %s\n\n"+
		"⚠️ This will immediately revert to the previous version!\n",
		appName, revIDStr, shortRev(revHash), instanceName)

	menu := &telebot.ReplyMarkup{}
	confirmData := fmt.Sprintf("%s|%s|%s", instanceName, appName, revIDStr)
	btnConfirm := menu.Data("🚨 ROLLBACK NOW", "argo_rollback_confirm", confirmData)
	btnCancel := menu.Data("❌ Cancel", "argo_rollback_cancel", fmt.Sprintf("%s|%s", instanceName, appName))
	menu.Inline(menu.Row(btnConfirm, btnCancel))

	_, err := c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// handleRollbackCancel cancels rollback and goes back to app detail.
func (m *Module) handleRollbackCancel(c telebot.Context) error {
	return m.handleAppDetail(c)
}

// handleRollbackConfirm executes the rollback.
func (m *Module) handleRollbackConfirm(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) != 3 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}
	instanceName, appName, revIDStr := parts[0], parts[1], parts[2]

	// Check approval requirement
	if m.approval != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		needsApproval, err := m.approval.CheckAndSubmit(ctx, c, approval.ApprovalInput{
			UserID:       user.TelegramID,
			Username:     user.Username,
			Action:       "argocd.apps.rollback",
			Resource:     appName,
			Cluster:      instanceName,
			Details:      map[string]interface{}{"revision": revIDStr, "instance": instanceName},
			CallbackData: c.Callback().Data,
		})
		if err != nil {
			m.logger.Error("approval check failed", zap.Error(err))
		}
		if needsApproval {
			return sendOrEdit(c, fmt.Sprintf("📋 Rollback for `%s` requires approval.\nWaiting for admin approval...", appName), nil)
		}
	}

	var revID int64
	fmt.Sscanf(revIDStr, "%d", &revID)

	_ = c.Respond(&telebot.CallbackResponse{Text: "🔄 Rolling back..."})

	rollbackCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inst, err := m.getInstanceByName(instanceName)
	if err != nil {
		return sendOrEdit(c, "⚠️ ArgoCD instance not found.", nil)
	}

	result, err := inst.client.RollbackApplication(rollbackCtx, appName, revID)

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
		Action:    "argocd.app.rollback",
		Resource:  fmt.Sprintf("app/%s", appName),
		Cluster:   instanceName,
		Status:    status,
		Error:     errMsg,
		Details:   map[string]interface{}{"revision": revID, "instance": instanceName},
		OccurredAt: time.Now().UTC(),
	})

	if err != nil {
		m.logger.Error("rollback failed",
			zap.String("app", appName),
			zap.String("instance", instanceName),
			zap.Int64("revision", revID),
			zap.Error(err),
		)
		return sendOrEdit(c, fmt.Sprintf("❌ Rollback failed for `%s`:\n%s", appName, err.Error()), nil)
	}

	// Disable auto-sync after rollback to prevent ArgoCD from syncing back to HEAD
	disableCtx, disableCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer disableCancel()
	if disableErr := inst.client.DisableAutoSync(disableCtx, appName); disableErr != nil {
		m.logger.Warn("failed to disable auto-sync after rollback",
			zap.String("app", appName),
			zap.Error(disableErr),
		)
	}

	text := formatRollbackResult(result, appName, revIDStr, user.Username)
	btnBack := &telebot.ReplyMarkup{}
	backData := fmt.Sprintf("%s|%s", instanceName, appName)
	btn := btnBack.Data("◀️ Back to App", "argo_app_detail", backData)
	btnBack.Inline(btnBack.Row(btn))

	_, editErr := c.Bot().Edit(c.Callback().Message, text, btnBack, telebot.ModeMarkdown)
	return editErr
}

// formatRollbackHistory formats the deployment history list.
func formatRollbackHistory(history []pkgargocd.RevisionHistory, instanceName, appName string) (string, *telebot.ReplyMarkup) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 *Deployment History — %s*\n", appName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	limit := len(history)
	if limit > maxHistoryItems {
		limit = maxHistoryItems
	}

	for i, h := range history[:limit] {
		label := "current"
		if i > 0 {
			label = "previous"
		}
		sb.WriteString(fmt.Sprintf("*Rev %d* — `%s`\n", h.ID, shortRev(h.Revision)))
		sb.WriteString(fmt.Sprintf("  %s | %s\n", label, h.DeployedAt.UTC().Format("2006-01-02 15:04")))
		if h.DeployedBy != "" {
			sb.WriteString(fmt.Sprintf("  By: %s\n", h.DeployedBy))
		}
		sb.WriteString("\n")

		if i > 0 { // Don't allow rolling back to current
			btnData := fmt.Sprintf("%s|%s|%d|%s", instanceName, appName, h.ID, h.Revision)
			btnLabel := fmt.Sprintf("Rev %d (%s)", h.ID, shortRev(h.Revision))
			btn := menu.Data(btnLabel, "argo_rollback_select", btnData)
			rows = append(rows, menu.Row(btn))
		}
	}

	rows = append(rows, menu.Row(
		menu.Data("❌ Cancel", "argo_rollback_cancel", fmt.Sprintf("%s|%s", instanceName, appName)),
	))
	menu.Inline(rows...)

	return sb.String(), menu
}

// formatRollbackResult formats the rollback result message.
func formatRollbackResult(result *pkgargocd.RollbackResult, appName, revID, triggeredBy string) string {
	phase := "succeeded"
	phaseEmoji := "✅"
	if result != nil && result.Phase != "" && result.Phase != "Succeeded" {
		phase = result.Phase
		phaseEmoji = "❌"
	}

	text := fmt.Sprintf("%s *Rollback Complete: %s*\n\n"+
		"Rolled back to Rev %s\n"+
		"Phase: %s\n\n"+
		"⚠️ _Auto-sync has been disabled to prevent ArgoCD from re-syncing to the newer revision._\n"+
		"Re-enable auto-sync manually when ready.\n\n"+
		"Triggered by: @%s at %s",
		phaseEmoji, appName, revID, phase,
		triggeredBy, time.Now().UTC().Format("2006-01-02 15:04:05"))

	if result != nil && result.Message != "" {
		text += fmt.Sprintf("\nMessage: %s", result.Message)
	}
	return text
}
