package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/rbac"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// handleNamespacesCommand handles the /namespaces slash command.
func (m *Module) handleNamespacesCommand(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesNamespacesList)
	if !allowed {
		return c.Send("⛔ You don't have permission to list namespaces.")
	}

	clusterName := m.userCtx.GetCluster(user.TelegramID)
	if clusterName == "" {
		return c.Send("⚠️ No cluster selected. Use /clusters to select one.")
	}

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		m.logger.Error("failed to connect to cluster",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to connect to cluster. Is it reachable?")
	}

	nsList, err := clientSet.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		m.logger.Error("failed to list namespaces",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to list namespaces.")
	}

	if len(nsList.Items) == 0 {
		return c.Send(fmt.Sprintf("📁 No namespaces found in cluster `%s`.", clusterName), telebot.ModeMarkdown)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📁 *Namespaces* (cluster: %s)\n", clusterName)
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

	activeCount := 0
	terminatingCount := 0

	for _, ns := range nsList.Items {
		emoji := "🟢"
		status := string(ns.Status.Phase)
		if status == "Terminating" {
			emoji = "🔴"
			terminatingCount++
		} else {
			activeCount++
		}

		age := time.Since(ns.CreationTimestamp.Time)
		fmt.Fprintf(&sb, "%s `%s` — %s (%s)\n", emoji, ns.Name, status, formatDuration(age))
	}

	fmt.Fprintf(&sb, "\n📊 Total: %d (%d Active, %d Terminating)",
		len(nsList.Items), activeCount, terminatingCount)

	menu := &telebot.ReplyMarkup{}
	btnRefresh := menu.Data("🔄 Refresh", "k8s_namespaces_refresh", clusterName)
	menu.Inline(menu.Row(btnRefresh))

	return c.Send(sb.String(), menu, telebot.ModeMarkdown)
}

// handleNamespacesRefresh refreshes the namespace list.
func (m *Module) handleNamespacesRefresh(c telebot.Context) error {
	clusterName := c.Callback().Data
	if clusterName == "" {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Cluster error"})
	}

	nsList, err := clientSet.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Failed to list namespaces"})
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📁 *Namespaces* (cluster: %s)\n", clusterName)
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

	activeCount := 0
	terminatingCount := 0

	for _, ns := range nsList.Items {
		emoji := "🟢"
		status := string(ns.Status.Phase)
		if status == "Terminating" {
			emoji = "🔴"
			terminatingCount++
		} else {
			activeCount++
		}

		age := time.Since(ns.CreationTimestamp.Time)
		fmt.Fprintf(&sb, "%s `%s` — %s (%s)\n", emoji, ns.Name, status, formatDuration(age))
	}

	fmt.Fprintf(&sb, "\n📊 Total: %d (%d Active, %d Terminating)",
		len(nsList.Items), activeCount, terminatingCount)

	menu := &telebot.ReplyMarkup{}
	btnRefresh := menu.Data("🔄 Refresh", "k8s_namespaces_refresh", clusterName)
	menu.Inline(menu.Row(btnRefresh))

	_, err = c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
	return err
}
