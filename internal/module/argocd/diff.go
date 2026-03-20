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

// handleDiff shows pending diffs for an ArgoCD application before sync.
func (m *Module) handleDiff(c telebot.Context) error {
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

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermArgoCDAppsDiff)
	if !allowed {
		return c.Respond(&telebot.CallbackResponse{Text: "⛔ You need operator+ role to view diffs"})
	}

	inst, err := m.getInstanceByName(instanceName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Instance not found"})
	}

	diff, err := inst.client.GetApplicationDiff(ctx, appName)
	if err != nil {
		m.logger.Error("failed to get argocd app diff",
			zap.String("instance", instanceName),
			zap.String("app", appName),
			zap.Error(err),
		)
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Failed to load diff"})
	}

	text := formatDiff(diff, instanceName)

	menu := &telebot.ReplyMarkup{}
	data := fmt.Sprintf("%s|%s", instanceName, appName)
	btnSync := menu.Data("⚡ Sync Now", "argo_sync", data)
	btnBack := menu.Data("◀️ Back", "argo_app_detail", data)
	menu.Inline(
		menu.Row(btnSync),
		menu.Row(btnBack),
	)

	_, err = c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// formatDiff formats a DiffResult for Telegram display.
func formatDiff(diff *pkgargocd.DiffResult, instanceName string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 *Diff — %s*\n", diff.AppName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("Instance: %s\n\n", instanceName))

	if diff.Changed == 0 && diff.Added == 0 && diff.Removed == 0 {
		sb.WriteString("✅ No pending changes — application is in sync.\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("📊 Summary: %d changed, %d added, %d removed\n\n",
		diff.Changed, diff.Added, diff.Removed))

	shown := 0
	for _, r := range diff.Resources {
		if r.Diff == "" {
			continue
		}
		if shown >= 15 {
			sb.WriteString(fmt.Sprintf("\n...and %d more resources\n", len(diff.Resources)-shown))
			break
		}
		emoji := "~"
		if r.Diff == "~ changed" {
			emoji = "🟡"
		}
		sb.WriteString(fmt.Sprintf("  %s `%s/%s`", emoji, r.Kind, r.Name))
		if r.Namespace != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", r.Namespace))
		}
		sb.WriteString("\n")
		shown++
	}

	sb.WriteString("\n═══════════════════\n")
	sb.WriteString("Use ⚡ Sync Now to apply these changes.\n")

	return sb.String()
}
