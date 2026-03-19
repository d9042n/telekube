package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/rbac"
	tfmt "github.com/d9042n/telekube/pkg/telegram"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// handleDeploysCommand handles the /deploys [namespace] slash command.
func (m *Module) handleDeploysCommand(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesDeploymentsList)
	if !allowed {
		return c.Send("⛔ You don't have permission to list deployments.")
	}

	clusterName := m.userCtx.GetCluster(user.TelegramID)
	if clusterName == "" {
		return c.Send("⚠️ No cluster selected. Use /clusters to select one.")
	}

	// Check if namespace provided as argument
	args := strings.Fields(c.Text())
	if len(args) >= 2 {
		namespace := args[1]
		return m.sendDeployList(c, clusterName, namespace)
	}

	// Show namespace selector
	namespaces, err := m.getNamespaces(ctx, clusterName)
	if err != nil {
		m.logger.Error("failed to list namespaces",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to connect to cluster. Is it reachable?")
	}

	markup := m.kb.NamespaceSelector(namespaces, "k8s_deploys")
	return c.Send(fmt.Sprintf("🚀 Select namespace for deployments (%s):", clusterName), markup)
}

// handleDeploysNamespaceSelect handles namespace selection for deployments.
func (m *Module) handleDeploysNamespaceSelect(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	namespace := c.Callback().Data
	clusterName := m.userCtx.GetCluster(user.TelegramID)

	return m.sendDeployList(c, clusterName, namespace)
}

// sendDeployList fetches and displays deployments for a namespace.
func (m *Module) sendDeployList(c telebot.Context, clusterName, namespace string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	nsLabel := namespace
	listNs := namespace
	if namespace == "_all" {
		nsLabel = "all namespaces"
		listNs = ""
	}

	deployList, err := clientSet.AppsV1().Deployments(listNs).List(ctx, metav1.ListOptions{})
	if err != nil {
		m.logger.Error("failed to list deployments",
			zap.String("cluster", clusterName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to list deployments.")
	}

	if len(deployList.Items) == 0 {
		return c.Send(fmt.Sprintf("🚀 No deployments found in %s (%s)", nsLabel, clusterName))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🚀 *Deployments in %s* (cluster: %s)\n", nsLabel, clusterName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

	for _, deploy := range deployList.Items {
		desired := *deploy.Spec.Replicas
		available := deploy.Status.AvailableReplicas
		ready := deploy.Status.ReadyReplicas

		// Determine status emoji
		emoji := "🟢"
		status := "Available"
		if available == 0 && desired > 0 {
			emoji = "🔴"
			status = "Degraded"
		} else if ready < desired {
			emoji = "🟡"
			status = "Progressing"
		}

		nsStr := ""
		if namespace == "_all" {
			nsStr = fmt.Sprintf(" [%s]", deploy.Namespace)
		}

		sb.WriteString(fmt.Sprintf("%s `%s`%s — %s\n", emoji, deploy.Name, nsStr, status))
		sb.WriteString(fmt.Sprintf("   Replicas: %d/%d ready, %d available\n\n",
			ready, desired, available))
	}

	// Build keyboard
	menu := &telebot.ReplyMarkup{}
	data := fmt.Sprintf("%s|%s", namespace, clusterName)
	btnRefresh := menu.Data("🔄 Refresh", "k8s_deploys_refresh", data)
	menu.Inline(menu.Row(btnRefresh))

	if c.Callback() != nil {
		_, err := c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
		return err
	}
	return c.Send(sb.String(), menu, telebot.ModeMarkdown)
}

// handleDeploysRefresh refreshes the deployment list.
func (m *Module) handleDeploysRefresh(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	namespace, clusterName := parts[0], parts[1]
	return m.sendDeployList(c, clusterName, namespace)
}

// Ensure tfmt is used (avoid unused import error).
var _ = tfmt.StatusEmoji
