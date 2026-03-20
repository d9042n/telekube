package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/rbac"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// Warning thresholds (configurable).
const (
	cpuHighThreshold     = 0.90 // 90%
	ramWarningThreshold  = 0.85 // 85%
	ramCriticalThreshold = 0.95 // 95%
	barWidth             = 10
	topPageSize          = 6
)

// renderBar renders a progress bar string.
func renderBar(used, total int64, width int) string {
	if total <= 0 {
		return "[" + strings.Repeat("░", width) + "]"
	}
	ratio := float64(used) / float64(total)
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * float64(width))
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

// thresholdEmoji returns a warning emoji based on usage ratio.
func thresholdEmoji(ratio float64, isCPU bool) string {
	if isCPU {
		if ratio >= cpuHighThreshold {
			return " 🔴 HIGH"
		}
		return ""
	}
	// RAM
	if ratio >= ramCriticalThreshold {
		return " 🔴 CRITICAL"
	}
	if ratio >= ramWarningThreshold {
		return " 🟡"
	}
	return ""
}

// handleTop handles the /top command — shows namespace selector for pod metrics.
func (m *Module) handleTop(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesMetricsView)
	if !allowed {
		return c.Send("⛔ You don't have permission to view metrics.")
	}

	// Check for /top nodes subcommand
	args := strings.Fields(c.Text())
	if len(args) >= 2 && strings.EqualFold(args[1], "nodes") {
		return m.sendNodeTop(c)
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
		return c.Send("⚠️ Failed to connect to cluster. Is it reachable?")
	}

	markup := m.kb.NamespaceSelector(namespaces, "k8s_top")
	return c.Send(fmt.Sprintf("📊 Select namespace for metrics (%s):", clusterName), markup)
}

// handleTopNamespaceSelect handles namespace selection for pod metrics.
func (m *Module) handleTopNamespaceSelect(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	namespace := c.Callback().Data
	clusterName := m.userCtx.GetCluster(user.TelegramID)

	return m.sendPodTop(c, clusterName, namespace, 1)
}

// sendPodTop fetches and displays pod metrics for a namespace.
func (m *Module) sendPodTop(c telebot.Context, clusterName, namespace string, page int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metricsClient, err := m.cluster.MetricsClient(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster metrics. Is Metrics Server installed?")
	}

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	// Fetch pod metrics
	var podMetrics *metricsv1beta1.PodMetricsList
	if namespace == "_all" {
		podMetrics, err = metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	} else {
		podMetrics, err = metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		m.logger.Warn("failed to get pod metrics (metrics-server may not be installed)",
			zap.String("cluster", clusterName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to get pod metrics. Is Metrics Server installed?\n\nInstall: kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml")
	}

	if len(podMetrics.Items) == 0 {
		nsLabel := namespace
		if namespace == "_all" {
			nsLabel = "all namespaces"
		}
		return c.Send(fmt.Sprintf("📊 No pod metrics found in %s (%s)", nsLabel, clusterName))
	}

	// Sort by CPU usage descending
	sort.Slice(podMetrics.Items, func(i, j int) bool {
		cpuI := int64(0)
		cpuJ := int64(0)
		for _, c := range podMetrics.Items[i].Containers {
			cpuI += c.Usage.Cpu().MilliValue()
		}
		for _, c := range podMetrics.Items[j].Containers {
			cpuJ += c.Usage.Cpu().MilliValue()
		}
		return cpuI > cpuJ
	})

	// Paginate
	totalItems := len(podMetrics.Items)
	totalPages := (totalItems + topPageSize - 1) / topPageSize
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * topPageSize
	end := start + topPageSize
	if end > totalItems {
		end = totalItems
	}

	nsLabel := namespace
	if namespace == "_all" {
		nsLabel = "all namespaces"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *Resource Usage — %s* (cluster: %s)\n", nsLabel, clusterName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	for _, pm := range podMetrics.Items[start:end] {
		totalCPU := int64(0)
		totalRAM := int64(0)
		for _, container := range pm.Containers {
			totalCPU += container.Usage.Cpu().MilliValue()
			totalRAM += container.Usage.Memory().Value()
		}

		// Try to get pod resource requests/limits for percentage calculation
		pod, podErr := clientSet.CoreV1().Pods(pm.Namespace).Get(ctx, pm.Name, metav1.GetOptions{})

		cpuLimit := int64(0)
		ramLimit := int64(0)
		if podErr == nil {
			for _, container := range pod.Spec.Containers {
				if lim := container.Resources.Limits.Cpu(); lim != nil {
					cpuLimit += lim.MilliValue()
				} else if req := container.Resources.Requests.Cpu(); req != nil {
					cpuLimit += req.MilliValue()
				}
				if lim := container.Resources.Limits.Memory(); lim != nil {
					ramLimit += lim.Value()
				} else if req := container.Resources.Requests.Memory(); req != nil {
					ramLimit += req.Value()
				}
			}
		}

		sb.WriteString(fmt.Sprintf("📦 `%s`\n", pm.Name))

		// CPU
		if cpuLimit > 0 {
			cpuRatio := float64(totalCPU) / float64(cpuLimit)
			sb.WriteString(fmt.Sprintf("   CPU %s %d%%   %dm / %dm%s\n",
				renderBar(totalCPU, cpuLimit, barWidth),
				int(cpuRatio*100),
				totalCPU, cpuLimit,
				thresholdEmoji(cpuRatio, true)))
		} else {
			sb.WriteString(fmt.Sprintf("   CPU: %dm (no limit set)\n", totalCPU))
		}

		// RAM
		if ramLimit > 0 {
			ramRatio := float64(totalRAM) / float64(ramLimit)
			sb.WriteString(fmt.Sprintf("   RAM %s %d%%   %s / %s%s\n",
				renderBar(totalRAM, ramLimit, barWidth),
				int(ramRatio*100),
				formatBytes(totalRAM), formatBytes(ramLimit),
				thresholdEmoji(ramRatio, false)))
		} else {
			sb.WriteString(fmt.Sprintf("   RAM: %s (no limit set)\n", formatBytes(totalRAM)))
		}
		sb.WriteString("\n")
	}

	// Build keyboard
	menu := &telebot.ReplyMarkup{}
	var actionBtns []telebot.Btn

	actionBtns = append(actionBtns, menu.Data("🔄 Refresh", "k8s_top_refresh", m.sd(fmt.Sprintf("%s|%s|%d", namespace, clusterName, page))))
	actionBtns = append(actionBtns, menu.Data("📊 Nodes", "k8s_top_nodes", clusterName))

	var rows []telebot.Row
	rows = append(rows, menu.Row(actionBtns...))

	// Pagination
	var navBtns []telebot.Btn
	if page > 1 {
		navBtns = append(navBtns, menu.Data("◀️ Prev", "k8s_top_page", m.sd(fmt.Sprintf("%s|%s|%d", namespace, clusterName, page-1))))
	}
	if page < totalPages {
		navBtns = append(navBtns, menu.Data("▶️ Next", "k8s_top_page", m.sd(fmt.Sprintf("%s|%s|%d", namespace, clusterName, page+1))))
	}
	if len(navBtns) > 0 {
		rows = append(rows, menu.Row(navBtns...))
	}

	rows = append(rows, menu.Row(
		menu.Data(fmt.Sprintf("📄 Page %d/%d", page, totalPages), "k8s_top_info"),
	))

	menu.Inline(rows...)

	if c.Callback() != nil {
		_, err := c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
		return err
	}
	return c.Send(sb.String(), menu, telebot.ModeMarkdown)
}

// handleTopPage handles pagination for pod metrics.
func (m *Module) handleTopPage(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) != 3 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	namespace, clusterName := parts[0], parts[1]
	var page int
	fmt.Sscanf(parts[2], "%d", &page)
	if page < 1 {
		page = 1
	}

	return m.sendPodTop(c, clusterName, namespace, page)
}

// handleTopRefresh refreshes pod metrics.
func (m *Module) handleTopRefresh(c telebot.Context) error {
	return m.handleTopPage(c)
}

// handleTopNodesCallback handles the "Nodes" button from pod top view.
func (m *Module) handleTopNodesCallback(c telebot.Context) error {
	return m.sendNodeTop(c)
}

// sendNodeTop fetches and displays node metrics.
func (m *Module) sendNodeTop(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		if c.Callback() != nil {
			return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
		}
		return c.Send("⚠️ Could not identify you.")
	}

	clusterName := m.userCtx.GetCluster(user.TelegramID)
	if clusterName == "" {
		return c.Send("⚠️ No cluster selected. Use /clusters to select one.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metricsClient, err := m.cluster.MetricsClient(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster metrics.")
	}

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	nodeMetrics, err := metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		m.logger.Warn("failed to get node metrics (metrics-server may not be installed)",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to get node metrics. Is Metrics Server installed?\n\nInstall: kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml")
	}

	// Get node info for capacity/conditions
	nodes, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return c.Send("⚠️ Failed to list nodes.")
	}

	nodeInfo := make(map[string]nodeStatus)
	for _, node := range nodes.Items {
		ready := "Unknown"
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" {
				if cond.Status == "True" {
					ready = "Ready"
				} else if cond.Status == "Unknown" {
					ready = "Unknown"
				} else {
					ready = "NotReady"
				}
			}
		}

		cpuCap := node.Status.Allocatable.Cpu().MilliValue()
		ramCap := node.Status.Allocatable.Memory().Value()

		nodeInfo[node.Name] = nodeStatus{
			ready:  ready,
			cpuCap: cpuCap,
			ramCap: ramCap,
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *Node Resource Usage* (cluster: %s)\n", clusterName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	for _, nm := range nodeMetrics.Items {
		totalCPU := nm.Usage.Cpu().MilliValue()
		totalRAM := nm.Usage.Memory().Value()

		info, ok := nodeInfo[nm.Name]
		statusEmoji := "⚪"
		statusText := "Unknown"
		if ok {
			statusText = info.ready
			switch info.ready {
			case "Ready":
				statusEmoji = "🟢"
			case "NotReady":
				statusEmoji = "🔴"
			}
		}

		sb.WriteString(fmt.Sprintf("🖥️ *%s* (%s %s)\n", nm.Name, statusEmoji, statusText))

		if ok && info.cpuCap > 0 {
			cpuRatio := float64(totalCPU) / float64(info.cpuCap)
			sb.WriteString(fmt.Sprintf("   CPU %s %d%%   %dm / %dm%s\n",
				renderBar(totalCPU, info.cpuCap, barWidth),
				int(cpuRatio*100),
				totalCPU, info.cpuCap,
				thresholdEmoji(cpuRatio, true)))
		} else {
			sb.WriteString(fmt.Sprintf("   CPU: %dm\n", totalCPU))
		}

		if ok && info.ramCap > 0 {
			ramRatio := float64(totalRAM) / float64(info.ramCap)
			sb.WriteString(fmt.Sprintf("   RAM %s %d%%   %s / %s%s\n",
				renderBar(totalRAM, info.ramCap, barWidth),
				int(ramRatio*100),
				formatBytes(totalRAM), formatBytes(info.ramCap),
				thresholdEmoji(ramRatio, false)))
		} else {
			sb.WriteString(fmt.Sprintf("   RAM: %s\n", formatBytes(totalRAM)))
		}
		sb.WriteString("\n")
	}

	menu := &telebot.ReplyMarkup{}
	btnRefresh := menu.Data("🔄 Refresh", "k8s_top_nodes_refresh", clusterName)
	btnBack := menu.Data("◀️ Back", "k8s_top_back", clusterName)
	menu.Inline(menu.Row(btnRefresh, btnBack))

	if c.Callback() != nil {
		_, err := c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
		return err
	}
	return c.Send(sb.String(), menu, telebot.ModeMarkdown)
}

// handleTopNodesRefresh refreshes node metrics.
func (m *Module) handleTopNodesRefresh(c telebot.Context) error {
	return m.sendNodeTop(c)
}

// handleTopBack goes back to namespace selection for /top.
func (m *Module) handleTopBack(c telebot.Context) error {
	return m.handleTop(c)
}

type nodeStatus struct {
	ready  string
	cpuCap int64
	ramCap int64
}

// formatBytes formats bytes into human-readable form.
func formatBytes(bytes int64) string {
	const (
		ki = 1024
		mi = ki * 1024
		gi = mi * 1024
	)
	switch {
	case bytes >= gi:
		return fmt.Sprintf("%.1fGi", float64(bytes)/float64(gi))
	case bytes >= mi:
		return fmt.Sprintf("%dMi", bytes/mi)
	case bytes >= ki:
		return fmt.Sprintf("%dKi", bytes/ki)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
