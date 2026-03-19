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

// handleQuota handles the /quota command — shows namespace selector.
func (m *Module) handleQuota(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesQuotaView)
	if !allowed {
		return c.Send("⛔ You don't have permission to view quotas.")
	}

	clusterName := m.userCtx.GetCluster(user.TelegramID)
	if clusterName == "" {
		return c.Send("⚠️ No cluster selected. Use /clusters to select one.")
	}

	namespaces, err := m.getNamespaces(ctx, clusterName)
	if err != nil {
		m.logger.Error("failed to list namespaces",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	markup := m.kb.NamespaceSelector(namespaces, "k8s_quota")
	return c.Send(fmt.Sprintf("📊 Select namespace for quotas (%s):", clusterName), markup)
}

// handleQuotaNamespaceSelect handles namespace selection for quotas.
func (m *Module) handleQuotaNamespaceSelect(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	namespace := c.Callback().Data
	clusterName := m.userCtx.GetCluster(user.TelegramID)

	return m.sendQuotaView(c, clusterName, namespace)
}

// sendQuotaView fetches and displays resource quotas.
func (m *Module) sendQuotaView(c telebot.Context, clusterName, namespace string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	if namespace == "_all" {
		return c.Send("⚠️ Please select a specific namespace for quota view.")
	}

	quotas, err := clientSet.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		m.logger.Error("failed to list resource quotas",
			zap.String("cluster", clusterName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to get resource quotas.")
	}

	if len(quotas.Items) == 0 {
		text := fmt.Sprintf("📊 *Resource Quotas — %s* (cluster: %s)\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nNo resource quotas configured.", namespace, clusterName)

		menu := &telebot.ReplyMarkup{}
		data := fmt.Sprintf("%s|%s", namespace, clusterName)
		btnRefresh := menu.Data("🔄 Refresh", "k8s_quota_refresh", data)
		btnBack := menu.Data("◀️ Back", "k8s_quota_back", clusterName)
		menu.Inline(menu.Row(btnRefresh, btnBack))

		if c.Callback() != nil {
			_, err := c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
			return err
		}
		return c.Send(text, menu, telebot.ModeMarkdown)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *Resource Quotas — %s* (cluster: %s)\n", namespace, clusterName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	var warnings []string

	for _, quota := range quotas.Items {
		for resourceName, hard := range quota.Status.Hard {
			used, ok := quota.Status.Used[resourceName]
			if !ok {
				continue
			}

			hardVal := hard.Value()
			usedVal := used.Value()

			// For CPU, use millivalue
			switch resourceName {
			case "requests.cpu", "limits.cpu", "cpu":
				hardVal = hard.MilliValue()
				usedVal = used.MilliValue()
			case "requests.memory", "limits.memory", "memory":
				// Keep byte values
			}

			if hardVal <= 0 {
				continue
			}

			ratio := float64(usedVal) / float64(hardVal)
			percentage := int(ratio * 100)

			var emoji string
			if ratio >= 0.95 {
				emoji = " 🔴"
				warnings = append(warnings, fmt.Sprintf("🔴 %s at %d%% — exceeding quota limit!", string(resourceName), percentage))
			} else if ratio >= 0.85 {
				emoji = " 🟡"
				warnings = append(warnings, fmt.Sprintf("⚠️ %s at %d%% — nearing quota limit!", string(resourceName), percentage))
			}

			var usedStr, hardStr string
			switch resourceName {
			case "requests.cpu", "limits.cpu", "cpu":
				usedStr = formatCPU(used.MilliValue())
				hardStr = formatCPU(hard.MilliValue())
			case "requests.memory", "limits.memory", "memory":
				usedStr = formatBytes(used.Value())
				hardStr = formatBytes(hard.Value())
			default:
				usedStr = used.String()
				hardStr = hard.String()
			}

			displayName := formatResourceName(string(resourceName))
			sb.WriteString(fmt.Sprintf("%s:  %s %d%%    %s / %s%s\n",
				displayName,
				renderBar(usedVal, hardVal, barWidth),
				percentage,
				usedStr, hardStr,
				emoji))
		}
	}

	if len(warnings) > 0 {
		sb.WriteString("\n")
		for _, w := range warnings {
			sb.WriteString(w + "\n")
		}
	}

	menu := &telebot.ReplyMarkup{}
	data := fmt.Sprintf("%s|%s", namespace, clusterName)
	btnRefresh := menu.Data("🔄 Refresh", "k8s_quota_refresh", data)
	btnBack := menu.Data("◀️ Back", "k8s_quota_back", clusterName)
	menu.Inline(menu.Row(btnRefresh, btnBack))

	if c.Callback() != nil {
		_, err := c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
		return err
	}
	return c.Send(sb.String(), menu, telebot.ModeMarkdown)
}

// handleQuotaRefresh refreshes quota view.
func (m *Module) handleQuotaRefresh(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	namespace, clusterName := parts[0], parts[1]
	return m.sendQuotaView(c, clusterName, namespace)
}

// handleQuotaBack goes back to namespace selector.
func (m *Module) handleQuotaBack(c telebot.Context) error {
	return m.handleQuota(c)
}

// formatResourceName makes resource names human-readable.
func formatResourceName(name string) string {
	switch name {
	case "requests.cpu":
		return "CPU Requests"
	case "limits.cpu":
		return "CPU Limits"
	case "requests.memory":
		return "RAM Requests"
	case "limits.memory":
		return "RAM Limits"
	case "pods":
		return "Pods"
	case "services":
		return "Services"
	case "persistentvolumeclaims":
		return "PVCs"
	case "configmaps":
		return "ConfigMaps"
	case "secrets":
		return "Secrets"
	default:
		return name
	}
}

// formatCPU formats CPU millivalue to human-readable string.
func formatCPU(milliValue int64) string {
	if milliValue >= 1000 {
		return fmt.Sprintf("%.1f cores", float64(milliValue)/1000)
	}
	return fmt.Sprintf("%dm", milliValue)
}
