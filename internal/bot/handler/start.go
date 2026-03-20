// Package handler provides Telegram bot command handlers.
package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/bot/keyboard"
	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/rbac"
	"gopkg.in/telebot.v3"
)

// Start handles the /start command.
func Start(clusterMgr cluster.Manager, userCtx *cluster.UserContext, rbacEngine rbac.Engine, kb *keyboard.Builder) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		user := middleware.GetUser(c)
		if user == nil {
			return c.Send("⚠️ Could not identify you. Please try again.")
		}

		clusters := clusterMgr.List()
		currentCluster := userCtx.GetCluster(user.TelegramID)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		role, _ := rbacEngine.GetRole(ctx, user.TelegramID)

		var sb strings.Builder
		sb.WriteString("🚀 *Welcome to Telekube!*\n\n")
		fmt.Fprintf(&sb, "👤 User: %s\n", user.DisplayName)
		fmt.Fprintf(&sb, "🔑 Role: %s\n", role)
		sb.WriteString(fmt.Sprintf("🌐 Current cluster: %s\n\n", currentCluster))
		sb.WriteString("Select your cluster:")

		markup := kb.ClusterSelector(clusters)
		return c.Send(sb.String(), markup, telebot.ModeMarkdown)
	}
}

// Help handles the /help command with dynamic content based on role.
func Help(registry *module.Registry, rbacEngine rbac.Engine) telebot.HandlerFunc {
	// Module name → display section header
	sectionHeaders := map[string]string{
		"kubernetes": "☸️ Kubernetes",
		"helm":      "⎈ Helm",
		"incident":  "🚨 Incident",
		"notify":    "🔔 Notifications",
		"rbac":      "🔐 RBAC",
		"argocd":    "🔄 ArgoCD",
		"approval":  "✅ Approval",
		"watcher":   "👀 Watcher",
		"briefing":  "📰 Briefing",
	}

	return func(c telebot.Context) error {
		user := middleware.GetUser(c)
		if user == nil {
			return c.Send("⚠️ Could not identify you.")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		var sb strings.Builder
		sb.WriteString("📋 *Telekube Commands*\n")
		sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━\n\n")

		// Core commands (always visible)
		sb.WriteString("🤖 *Core*\n")
		sb.WriteString("  /start — Welcome & cluster selection\n")
		sb.WriteString("  /help — This help message\n")
		sb.WriteString("  /clusters — Switch cluster\n")
		sb.WriteString("  /audit — View audit log\n")
		sb.WriteString("\n")

		// Module commands grouped by module
		for _, mc := range registry.ModulesWithCommands() {
			header, ok := sectionHeaders[mc.Name]
			if !ok {
				name := mc.Name
				if len(name) > 0 {
					name = strings.ToUpper(name[:1]) + name[1:]
				}
				header = "📦 " + name
			}

			// Filter by permission
			var visible []module.CommandInfo
			for _, cmd := range mc.Commands {
				if cmd.Permission != "" {
					allowed, _ := rbacEngine.HasPermission(ctx, user.TelegramID, cmd.Permission)
					if !allowed {
						continue
					}
				}
				visible = append(visible, cmd)
			}

			if len(visible) == 0 {
				continue
			}

			sb.WriteString(fmt.Sprintf("*%s*\n", header))
			for _, cmd := range visible {
				sb.WriteString(fmt.Sprintf("  %s — %s\n", cmd.Command, cmd.Description))
			}
			sb.WriteString("\n")
		}

		// Background features
		sb.WriteString("*⚙️ Background*\n")
		sb.WriteString("  👀 Watcher — Pod/Node/CronJob/Cert/PVC auto-alerts\n")
		sb.WriteString("  🛡 AlertManager — Webhook receiver\n")
		sb.WriteString("  ✅ Approval — Gate for dangerous ops\n")
		sb.WriteString("\n")

		sb.WriteString("_Use buttons for interactive navigation!_ 🎛")

		return c.Send(sb.String(), telebot.ModeMarkdown)
	}
}



// Clusters handles the /clusters command.
func Clusters(clusterMgr cluster.Manager, userCtx *cluster.UserContext, kb *keyboard.Builder) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		user := middleware.GetUser(c)
		if user == nil {
			return c.Send("⚠️ Could not identify you.")
		}

		clusters := clusterMgr.List()
		currentCluster := userCtx.GetCluster(user.TelegramID)

		var sb strings.Builder
		sb.WriteString("🌐 *Clusters*\n")
		sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

		for _, cl := range clusters {
			marker := ""
			if cl.Name == currentCluster {
				marker = " ← current"
			}
			sb.WriteString(fmt.Sprintf("%s %s%s\n", cl.Status.Emoji(), cl.DisplayName, marker))
		}

		sb.WriteString("\nSelect a cluster:")

		markup := kb.ClusterSelector(clusters)
		return c.Send(sb.String(), markup, telebot.ModeMarkdown)
	}
}

// ClusterSelect handles cluster selection callback.
func ClusterSelect(clusterMgr cluster.Manager, userCtx *cluster.UserContext, kb *keyboard.Builder) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		user := middleware.GetUser(c)
		if user == nil {
			return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
		}

		clusterName := c.Callback().Data
		if clusterName == "" {
			return c.Respond(&telebot.CallbackResponse{Text: "⚠️ No cluster selected"})
		}

		// Validate cluster exists
		info, err := clusterMgr.Get(clusterName)
		if err != nil {
			return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Cluster not found"})
		}

		userCtx.SetCluster(user.TelegramID, clusterName)

		// Send toast notification
		_ = c.Respond(&telebot.CallbackResponse{
			Text: fmt.Sprintf("✅ Switched to %s", info.DisplayName),
		})

		// Update the original message with selected cluster marked
		clusters := clusterMgr.List()

		var sb strings.Builder
		sb.WriteString("🌐 *Clusters*\n")
		sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

		for _, cl := range clusters {
			if cl.Name == clusterName {
				sb.WriteString(fmt.Sprintf("✅ *%s*  ← selected\n", cl.DisplayName))
			} else {
				sb.WriteString(fmt.Sprintf("%s %s\n", cl.Status.Emoji(), cl.DisplayName))
			}
		}

		sb.WriteString(fmt.Sprintf("\n🔗 Connected to *%s*", info.DisplayName))

		markup := kb.ClusterSelector(clusters)
		return c.Edit(sb.String(), markup, telebot.ModeMarkdown)
	}
}
