package approval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/rbac"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// BotModule wraps Manager and registers Telegram handlers.
type BotModule struct {
	manager *Manager
	logger  *zap.Logger
}

// NewBotModule creates a BotModule.
func NewBotModule(manager *Manager, logger *zap.Logger) *BotModule {
	return &BotModule{manager: manager, logger: logger}
}

func (m *BotModule) Name() string        { return "approval" }
func (m *BotModule) Description() string { return "Approval workflow for dangerous operations" }

func (m *BotModule) Register(bot *telebot.Bot, _ *telebot.Group) {
	bot.Handle(&telebot.Btn{Unique: "appr_approve"}, m.handleApprove)
	bot.Handle(&telebot.Btn{Unique: "appr_reject"}, m.handleReject)
	bot.Handle(&telebot.Btn{Unique: "appr_cancel"}, m.handleCancel)
}

func (m *BotModule) Start(ctx context.Context) error {
	m.manager.StartExpiryWorker(ctx)
	m.logger.Info("approval module started")
	return nil
}

func (m *BotModule) Stop(_ context.Context) error {
	m.logger.Info("approval module stopped")
	return nil
}

func (m *BotModule) Health() entity.HealthStatus { return entity.HealthStatusHealthy }

func (m *BotModule) Commands() []module.CommandInfo {
	return nil // No slash commands — approval is triggered by other modules
}

func (m *BotModule) handleApprove(c telebot.Context) error {
	data := c.Callback().Data
	return m.recordDecision(c, data, "approved", "")
}

func (m *BotModule) handleReject(c telebot.Context) error {
	data := c.Callback().Data
	return m.recordDecision(c, data, "rejected", "")
}

func (m *BotModule) handleCancel(c telebot.Context) error {
	requestID := c.Callback().Data
	ctx := context.Background()
	userID := c.Sender().ID
	if err := m.manager.Cancel(ctx, requestID, userID); err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: fmt.Sprintf("❌ %s", err.Error())})
	}
	return c.Respond(&telebot.CallbackResponse{Text: "✅ Request cancelled"})
}

func (m *BotModule) recordDecision(c telebot.Context, requestID, decision, comment string) error {
	ctx := context.Background()
	approver := c.Sender()

	approved, err := m.manager.Decide(ctx, requestID, approver.ID,
		approver.Username, decision, comment)
	if err != nil {
		_ = c.Respond(&telebot.CallbackResponse{Text: fmt.Sprintf("❌ %s", err.Error())})
		return nil
	}

	emoji := "✅"
	if decision == "rejected" {
		emoji = "❌"
	}
	text := fmt.Sprintf("%s Decision recorded", emoji)
	if approved {
		text = "✅ Approved — executing action..."
	}

	_ = c.Respond(&telebot.CallbackResponse{Text: text})

	req, err := m.manager.GetByID(ctx, requestID)
	if err != nil {
		return nil
	}

	// Edit the original approval message.
	if req.MessageID > 0 {
			_, _ = c.Bot().Edit(
				&telebot.Message{ID: req.MessageID, Chat: &telebot.Chat{ID: req.ChatID}},
				formatResolutionMessage(req),
			)
		}
	return nil
}

// formatResolutionMessage formats the resolved approval message.
func formatResolutionMessage(req *entity.ApprovalRequest) string {
	emoji := "✅ APPROVED"
	if req.Status == entity.ApprovalRejected {
		emoji = "❌ REJECTED"
	} else if req.Status == entity.ApprovalCancelled {
		emoji = "🚫 CANCELLED"
	} else if req.Status == entity.ApprovalExpired {
		emoji = "⏰ EXPIRED"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s — Request #%s\n", emoji, req.ID[:8]))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString(fmt.Sprintf("🎯 Action:  %s\n", req.Action))
	sb.WriteString(fmt.Sprintf("📦 Resource: %s\n", req.Resource))
	if req.Cluster != "" {
		sb.WriteString(fmt.Sprintf("📍 Cluster: %s\n", req.Cluster))
	}

	for _, a := range req.Approvers {
		if a.Decision != "pending" {
			decEmoji := "✅"
			if a.Decision == "rejected" {
				decEmoji = "❌"
			}
			sb.WriteString(fmt.Sprintf("\n%s By: @%d\n", decEmoji, a.UserID))
			if a.Comment != "" {
				sb.WriteString(fmt.Sprintf("   Reason: %s\n", a.Comment))
			}
		}
	}
	return sb.String()
}

// BuildApprovalMessage creates the message shown to approvers.
func BuildApprovalMessage(req *entity.ApprovalRequest) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 APPROVAL REQUEST #%s\n", req.ID[:8]))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString(fmt.Sprintf("👤 Requester: @%s\n", req.RequesterName))
	sb.WriteString(fmt.Sprintf("🎯 Action:    %s\n", req.Action))
	sb.WriteString(fmt.Sprintf("📦 Resource:  %s\n", req.Resource))
	if req.Cluster != "" {
		sb.WriteString(fmt.Sprintf("📍 Cluster:   %s\n", req.Cluster))
	}
	if req.Namespace != "" {
		sb.WriteString(fmt.Sprintf("🏷️  Namespace: %s\n", req.Namespace))
	}
	sb.WriteString(fmt.Sprintf("\nRequires: %d approval(s)\n", req.RequiredApprovals))
	sb.WriteString(fmt.Sprintf("Expires:  %s (%.0f minutes)\n",
		req.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC"),
		time.Until(req.ExpiresAt).Minutes(),
	))
	return sb.String()
}

// ApprovalKeyboard returns the inline keyboard for an approval request.
func ApprovalKeyboard(req *entity.ApprovalRequest) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}
	approveBtn := menu.Data("✅ Approve", "appr_approve", req.ID)
	rejectBtn := menu.Data("❌ Reject", "appr_reject", req.ID)
	cancelBtn := menu.Data("🚫 Cancel", "appr_cancel", req.ID)
	menu.Inline(
		menu.Row(approveBtn, rejectBtn),
		menu.Row(cancelBtn),
	)
	return menu
}

// RequiredPermission returns the rbac permission name for an action.
func RequiredPermission(action string) string {
	parts := strings.SplitN(action, ".", 3)
	if len(parts) == 3 {
		return action
	}
	return rbac.PermAdminRBACManage
}
