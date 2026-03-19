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
		sb.WriteString(fmt.Sprintf("👤 User: %s\n", user.DisplayName))
		sb.WriteString(fmt.Sprintf("🔑 Role: %s\n", role))
		sb.WriteString(fmt.Sprintf("🌐 Current cluster: %s\n\n", currentCluster))
		sb.WriteString("Select your cluster:")

		markup := kb.ClusterSelector(clusters)
		return c.Send(sb.String(), markup, telebot.ModeMarkdown)
	}
}

// Help handles the /help command with dynamic content based on role.
func Help(registry interface{ AllCommands() []module.CommandInfo }, rbacEngine rbac.Engine) telebot.HandlerFunc {
	type cmdProvider interface {
		AllCommands() []module.CommandInfo
	}
	return func(c telebot.Context) error {
		user := middleware.GetUser(c)
		if user == nil {
			return c.Send("⚠️ Could not identify you.")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		var sb strings.Builder
		sb.WriteString("📋 *Available Commands*\n")
		sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

		// Base commands
		sb.WriteString("🤖 *General:*\n")
		sb.WriteString("  /start — Welcome & cluster selection\n")
		sb.WriteString("  /help — This help message\n")
		sb.WriteString("  /clusters — Switch cluster\n")
		sb.WriteString("\n")

		// Module commands (filtered by permission)
		if p, ok := registry.(cmdProvider); ok {
			cmds := p.AllCommands()
			groups := make(map[string][]string) // group -> commands
			for _, cmd := range cmds {
				// Check permission
				if cmd.Permission != "" {
					allowed, _ := rbacEngine.HasPermission(ctx, user.TelegramID, cmd.Permission)
					if !allowed {
						continue
					}
				}
				// Group by first word of command
				group := "Other"
				if strings.HasPrefix(cmd.Command, "/pods") || strings.HasPrefix(cmd.Command, "/logs") ||
					strings.HasPrefix(cmd.Command, "/events") || strings.HasPrefix(cmd.Command, "/restart") {
					group = "📦 Kubernetes"
				} else if strings.HasPrefix(cmd.Command, "/audit") {
					group = "🔧 Admin"
				}
				groups[group] = append(groups[group], fmt.Sprintf("  %s — %s", cmd.Command, cmd.Description))
			}

			for group, cmds := range groups {
				sb.WriteString(fmt.Sprintf("*%s:*\n", group))
				for _, cmd := range cmds {
					sb.WriteString(cmd + "\n")
				}
				sb.WriteString("\n")
			}
		}

		sb.WriteString("Use buttons for interactive navigation! 🎛")

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
func ClusterSelect(clusterMgr cluster.Manager, userCtx *cluster.UserContext) telebot.HandlerFunc {
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

		return c.Respond(&telebot.CallbackResponse{
			Text: fmt.Sprintf("✅ Switched to %s", info.DisplayName),
		})
	}
}
