package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/d9042n/telekube/internal/storage"
	"gopkg.in/telebot.v3"
)

// AuditLog handles the /audit command.
func AuditLog(auditLogger audit.Logger, rbacEngine rbac.Engine) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		user := middleware.GetUser(c)
		if user == nil {
			return c.Send("⚠️ Could not identify you.")
		}

		// Check admin permission
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		allowed, err := rbacEngine.HasPermission(ctx, user.TelegramID, rbac.PermAdminAuditView)
		if err != nil || !allowed {
			return c.Send("⛔ You don't have permission to view the audit log.")
		}

		// Query last 24h
		now := time.Now().UTC()
		from := now.Add(-24 * time.Hour)
		filter := storage.AuditFilter{
			From:     &from,
			Page:     1,
			PageSize: 10,
		}

		entries, total, err := auditLogger.Query(ctx, filter)
		if err != nil {
			return c.Send("⚠️ Failed to load audit log.")
		}

		var sb strings.Builder
		sb.WriteString("📜 *Recent Actions* (last 24h)\n")
		sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

		if len(entries) == 0 {
			sb.WriteString("No actions recorded in the last 24 hours.")
		} else {
			for _, e := range entries {
				emoji := actionEmoji(e.Status)
				timeStr := e.OccurredAt.Format("15:04")
				username := e.Username
				if username == "" {
					username = fmt.Sprintf("id:%d", e.UserID)
				}
				action := e.Action
				if len(action) > 40 {
					action = action[:40] + "..."
				}

				line := fmt.Sprintf("%s %s @%s — %s", emoji, timeStr, username, action)
				if e.Cluster != "" {
					line += fmt.Sprintf(" (%s)", e.Cluster)
				}
				if e.Status != "" {
					line += " " + statusLabel(e.Status)
				}
				sb.WriteString(line + "\n")
			}

			sb.WriteString(fmt.Sprintf("\nShowing %d of %d entries", len(entries), total))
		}

		return c.Send(sb.String(), telebot.ModeMarkdown)
	}
}

func actionEmoji(status string) string {
	switch status {
	case "success":
		return "✅"
	case "denied":
		return "⛔"
	case "error":
		return "❌"
	default:
		return "ℹ️"
	}
}

func statusLabel(status string) string {
	switch status {
	case "success":
		return "✅"
	case "denied":
		return "DENIED"
	case "error":
		return "ERROR"
	default:
		return ""
	}
}
