package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module/approval"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

const (
	drainTimeout   = 5 * time.Minute
	drainGraceSecs = int64(30)
)

// handleNodes handles the /nodes command — lists all nodes.
func (m *Module) handleNodes(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesNodesView)
	if !allowed {
		return c.Send("⛔ You don't have permission to view nodes.")
	}

	return m.sendNodeList(c)
}

// sendNodeList display the node list.
func (m *Module) sendNodeList(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	clusterName := m.userCtx.GetCluster(user.TelegramID)
	if clusterName == "" {
		return c.Send("⚠️ No cluster selected. Use /clusters to select one.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	nodes, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		m.logger.Error("failed to list nodes",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to list nodes.")
	}

	// Try to get metrics
	metricsClient, _ := m.cluster.MetricsClient(clusterName)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🖥️ *Nodes* (cluster: %s)\n", clusterName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	for _, node := range nodes.Items {
		statusEmoji, statusText := nodeConditionDisplay(&node)

		// Count pods on this node
		podCount := 0
		pods, listErr := clientSet.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("spec.nodeName", node.Name).String(),
		})
		if listErr == nil {
			podCount = len(pods.Items)
		}

		// Try to get metrics
		cpuStr := "--"
		ramStr := "--"
		if metricsClient != nil {
			nm, nmErr := metricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, node.Name, metav1.GetOptions{})
			if nmErr == nil {
				cpuTotal := nm.Usage.Cpu().MilliValue()
				cpuCap := node.Status.Allocatable.Cpu().MilliValue()
				ramTotal := nm.Usage.Memory().Value()
				ramCap := node.Status.Allocatable.Memory().Value()

				if cpuCap > 0 {
					cpuStr = fmt.Sprintf("%d%%", int(float64(cpuTotal)/float64(cpuCap)*100))
				}
				if ramCap > 0 {
					ramStr = fmt.Sprintf("%d%%", int(float64(ramTotal)/float64(ramCap)*100))
				}
			}
		}

		sb.WriteString(fmt.Sprintf("%s `%s`  %s  %d pods  CPU: %s  RAM: %s\n",
			statusEmoji, node.Name, statusText, podCount, cpuStr, ramStr))

		data := fmt.Sprintf("%s|%s", node.Name, clusterName)
		btn := menu.Data(
			fmt.Sprintf("%s %s", statusEmoji, node.Name),
			"k8s_node_detail",
			data,
		)
		rows = append(rows, menu.Row(btn))
	}

	rows = append(rows, menu.Row(
		menu.Data("📊 Top Nodes", "k8s_top_nodes", clusterName),
		menu.Data("🔄 Refresh", "k8s_nodes_refresh", clusterName),
	))

	menu.Inline(rows...)

	if c.Callback() != nil {
		_, err := c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
		return err
	}
	return c.Send(sb.String(), menu, telebot.ModeMarkdown)
}

// handleNodesRefresh refreshes the node list.
func (m *Module) handleNodesRefresh(c telebot.Context) error {
	return m.sendNodeList(c)
}

// handleNodeDetail shows detailed node information.
func (m *Module) handleNodeDetail(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	nodeName, clusterName := parts[0], parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Cluster error"})
	}

	node, err := clientSet.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Node not found"})
	}

	statusEmoji, statusText := nodeConditionDisplay(node)

	// Pod count
	pods, _ := clientSet.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", nodeName).String(),
	})
	podCount := 0
	if pods != nil {
		podCount = len(pods.Items)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🖥️ *%s*\n", nodeName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("Status:     %s %s\n", statusEmoji, statusText))
	sb.WriteString(fmt.Sprintf("Pods:       %d\n", podCount))

	// Resource info
	cpuCap := node.Status.Allocatable.Cpu().MilliValue()
	ramCap := node.Status.Allocatable.Memory().Value()

	// Try node metrics
	metricsClient, _ := m.cluster.MetricsClient(clusterName)
	if metricsClient != nil {
		nm, nmErr := metricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, nodeName, metav1.GetOptions{})
		if nmErr == nil {
			cpuUsed := nm.Usage.Cpu().MilliValue()
			ramUsed := nm.Usage.Memory().Value()

			if cpuCap > 0 {
				cpuRatio := float64(cpuUsed) / float64(cpuCap)
				sb.WriteString(fmt.Sprintf("CPU:        %dm / %dm (%d%%)\n", cpuUsed, cpuCap, int(cpuRatio*100)))
			}
			if ramCap > 0 {
				ramRatio := float64(ramUsed) / float64(ramCap)
				sb.WriteString(fmt.Sprintf("RAM:        %s / %s (%d%%)\n", formatBytes(ramUsed), formatBytes(ramCap), int(ramRatio*100)))
			}
		}
	}

	// Node metadata
	sb.WriteString(fmt.Sprintf("Kubelet:    %s\n", node.Status.NodeInfo.KubeletVersion))
	sb.WriteString(fmt.Sprintf("OS:         %s %s\n", node.Status.NodeInfo.OperatingSystem, node.Status.NodeInfo.OSImage))

	// Buttons
	menu := &telebot.ReplyMarkup{}
	data := fmt.Sprintf("%s|%s", nodeName, clusterName)

	var actionBtns []telebot.Btn

	user := middleware.GetUser(c)
	if user != nil {
		cordonCtx, cordonCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cordonCancel()

		canCordon, _ := m.rbac.HasPermission(cordonCtx, user.TelegramID, rbac.PermKubernetesNodesCordon)
		canDrain, _ := m.rbac.HasPermission(cordonCtx, user.TelegramID, rbac.PermKubernetesNodesDrain)

		if canCordon {
			if node.Spec.Unschedulable {
				actionBtns = append(actionBtns, menu.Data("🔓 Uncordon", "k8s_node_uncordon", data))
			} else {
				actionBtns = append(actionBtns, menu.Data("🔒 Cordon", "k8s_node_cordon", data))
			}
		}
		if canDrain {
			actionBtns = append(actionBtns, menu.Data("📤 Drain", "k8s_node_drain", data))
		}
	}

	actionBtns = append(actionBtns, menu.Data("📊 Top Pods", "k8s_node_top_pods", data))

	var rows []telebot.Row
	if len(actionBtns) > 0 {
		rows = append(rows, menu.Row(actionBtns...))
	}
	rows = append(rows, menu.Row(
		menu.Data("◀️ Back", "k8s_nodes_back", clusterName),
	))

	menu.Inline(rows...)

	_, err = c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
	return err
}

// handleNodeCordon shows confirmation for cordoning a node.
func (m *Module) handleNodeCordon(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	nodeName, clusterName := parts[0], parts[1]

	text := fmt.Sprintf("⚠️ *Cordon %s?*\n\nThis will prevent new pods from being scheduled.\nExisting pods will NOT be affected.\n\nCluster: %s",
		nodeName, clusterName)

	menu := &telebot.ReplyMarkup{}
	data := c.Callback().Data
	btnConfirm := menu.Data("✅ Confirm", "k8s_node_cordon_confirm", data)
	btnCancel := menu.Data("❌ Cancel", "k8s_node_detail", data)
	menu.Inline(menu.Row(btnConfirm, btnCancel))

	_, err := c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// handleNodeCordonConfirm executes the cordon operation.
func (m *Module) handleNodeCordonConfirm(c telebot.Context) error {
	return m.performCordon(c, true)
}

// handleNodeUncordon shows confirmation for uncordoning.
func (m *Module) handleNodeUncordon(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	nodeName, clusterName := parts[0], parts[1]

	text := fmt.Sprintf("⚠️ *Uncordon %s?*\n\nThis will allow new pods to be scheduled on this node.\n\nCluster: %s",
		nodeName, clusterName)

	menu := &telebot.ReplyMarkup{}
	data := c.Callback().Data
	btnConfirm := menu.Data("✅ Confirm", "k8s_node_uncordon_confirm", data)
	btnCancel := menu.Data("❌ Cancel", "k8s_node_detail", data)
	menu.Inline(menu.Row(btnConfirm, btnCancel))

	_, err := c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// handleNodeUncordonConfirm executes the uncordon operation.
func (m *Module) handleNodeUncordonConfirm(c telebot.Context) error {
	return m.performCordon(c, false)
}

// performCordon cordons or uncordons a node.
func (m *Module) performCordon(c telebot.Context, cordon bool) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	nodeName, clusterName := parts[0], parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Cluster error"})
	}

	node, err := clientSet.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Node not found"})
	}

	node.Spec.Unschedulable = cordon
	_, err = clientSet.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		m.logger.Error("failed to cordon/uncordon node",
			zap.String("node", nodeName),
			zap.Bool("cordon", cordon),
			zap.Error(err),
		)

		action := "node.cordon"
		if !cordon {
			action = "node.uncordon"
		}
		m.audit.Log(entity.AuditEntry{
			ID:         ulid.Make().String(),
			UserID:     user.TelegramID,
			Username:   user.Username,
			Action:     action,
			Resource:   fmt.Sprintf("node/%s", nodeName),
			Cluster:    clusterName,
			Status:     entity.AuditStatusError,
			Error:      err.Error(),
			OccurredAt: time.Now().UTC(),
		})

		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Failed to update node"})
	}

	action := "node.cordon"
	statusText := "cordoned (SchedulingDisabled)"
	if !cordon {
		action = "node.uncordon"
		statusText = "uncordoned (Scheduling resumed)"
	}

	m.audit.Log(entity.AuditEntry{
		ID:         ulid.Make().String(),
		UserID:     user.TelegramID,
		Username:   user.Username,
		Action:     action,
		Resource:   fmt.Sprintf("node/%s", nodeName),
		Cluster:    clusterName,
		Status:     entity.AuditStatusSuccess,
		OccurredAt: time.Now().UTC(),
	})

	text := fmt.Sprintf("✅ Node `%s` %s\n\nTriggered by: @%s at %s",
		nodeName, statusText, user.Username, time.Now().UTC().Format("2006-01-02 15:04:05"))

	menu := &telebot.ReplyMarkup{}
	data := fmt.Sprintf("%s|%s", nodeName, clusterName)
	btnDetail := menu.Data("🖥️ Node Details", "k8s_node_detail", data)
	btnList := menu.Data("◀️ All Nodes", "k8s_nodes_back", clusterName)
	menu.Inline(menu.Row(btnDetail, btnList))

	_, err = c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// handleNodeDrain shows confirmation for draining a node.
func (m *Module) handleNodeDrain(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	nodeName, clusterName := parts[0], parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Cluster error"})
	}

	// Count pods that will be evicted
	pods, _ := clientSet.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", nodeName).String(),
	})
	evictableCount := 0
	if pods != nil {
		for _, pod := range pods.Items {
			if !isDaemonSetPod(&pod) {
				evictableCount++
			}
		}
	}

	text := fmt.Sprintf("⚠️ *Drain %s?*\n\nThis will:\n• Cordon the node (no new pods)\n• Gracefully evict %d pods\n• DaemonSet pods will NOT be evicted\n\n⏱️ Grace period: %d seconds\n\nCluster: %s",
		nodeName, evictableCount, drainGraceSecs, clusterName)

	menu := &telebot.ReplyMarkup{}
	data := c.Callback().Data
	btnConfirm := menu.Data("✅ Confirm Drain", "k8s_node_drain_confirm", data)
	btnCancel := menu.Data("❌ Cancel", "k8s_node_detail", data)
	menu.Inline(menu.Row(btnConfirm, btnCancel))

	_, err = c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// handleNodeDrainConfirm executes the drain operation.
func (m *Module) handleNodeDrainConfirm(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	nodeName, clusterName := parts[0], parts[1]

	// Check approval requirement
	if m.approval != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		needsApproval, err := m.approval.CheckAndSubmit(ctx, c, approval.ApprovalInput{
			UserID:       user.TelegramID,
			Username:     user.Username,
			Action:       "kubernetes.nodes.drain",
			Resource:     nodeName,
			Cluster:      clusterName,
			Details:      map[string]interface{}{"node": nodeName},
			CallbackData: c.Callback().Data,
		})
		if err != nil {
			m.logger.Error("approval check failed", zap.Error(err))
		}
		if needsApproval {
			return c.Respond(&telebot.CallbackResponse{
				Text: fmt.Sprintf("📋 Drain %s requires approval. Request submitted.", nodeName),
			})
		}
	}

	// Show draining message
	text := fmt.Sprintf("🔄 *Draining %s...*\n\nCordoning node first...", nodeName)
	_, _ = c.Bot().Edit(c.Callback().Message, text, telebot.ModeMarkdown)

	// Run drain in background
	go m.performDrain(c, user, nodeName, clusterName)

	return nil
}

// performDrain performs the full drain operation (cordon + evict).
func (m *Module) performDrain(c telebot.Context, user *entity.User, nodeName, clusterName string) {
	ctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		m.updateDrainMessage(c, fmt.Sprintf("⚠️ Failed to drain %s: cluster error", nodeName))
		return
	}

	// Step 1: Cordon
	node, err := clientSet.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		m.updateDrainMessage(c, fmt.Sprintf("⚠️ Failed to drain %s: node not found", nodeName))
		return
	}

	node.Spec.Unschedulable = true
	_, err = clientSet.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		m.updateDrainMessage(c, fmt.Sprintf("⚠️ Failed to cordon %s: %s", nodeName, err.Error()))
		return
	}

	// Step 2: List pods to evict
	pods, err := clientSet.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", nodeName).String(),
	})
	if err != nil {
		m.updateDrainMessage(c, fmt.Sprintf("⚠️ Failed to list pods on %s: %s", nodeName, err.Error()))
		return
	}

	// Step 3: Evict pods one by one
	evicted := 0
	skipped := 0
	failed := 0
	total := 0

	var evictLog strings.Builder
	evictLog.WriteString(fmt.Sprintf("🔄 *Draining %s...*\n\n", nodeName))

	for _, pod := range pods.Items {
		if isDaemonSetPod(&pod) {
			skipped++
			continue
		}
		total++
	}

	for _, pod := range pods.Items {
		if isDaemonSetPod(&pod) {
			continue
		}

		evictLog.WriteString(fmt.Sprintf("Evicting `%s`...", pod.Name))

		gracePeriod := drainGraceSecs
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
			DeleteOptions: &metav1.DeleteOptions{
				GracePeriodSeconds: &gracePeriod,
			},
		}

		evictErr := clientSet.CoreV1().Pods(pod.Namespace).EvictV1(ctx, eviction)
		if evictErr != nil {
			evictLog.WriteString(" ❌\n")
			failed++
			m.logger.Error("failed to evict pod",
				zap.String("pod", pod.Name),
				zap.String("node", nodeName),
				zap.Error(evictErr),
			)
		} else {
			evictLog.WriteString(" ✅\n")
			evicted++
		}

		// Update progress
		m.updateDrainMessage(c, evictLog.String())
	}

	// Final status
	status := entity.AuditStatusSuccess
	statusEmoji := "✅"
	if failed > 0 {
		status = entity.AuditStatusError
		statusEmoji = "⚠️"
	}

	m.audit.Log(entity.AuditEntry{
		ID:         ulid.Make().String(),
		UserID:     user.TelegramID,
		Username:   user.Username,
		Action:     "node.drain",
		Resource:   fmt.Sprintf("node/%s", nodeName),
		Cluster:    clusterName,
		Status:     status,
		Details: map[string]interface{}{
			"evicted": evicted,
			"skipped": skipped,
			"failed":  failed,
		},
		OccurredAt: time.Now().UTC(),
	})

	finalText := fmt.Sprintf("%s Node `%s` drained\n\n✅ Evicted: %d\n⏭️ Skipped (DaemonSet): %d\n❌ Failed: %d\n\nTriggered by: @%s at %s",
		statusEmoji, nodeName, evicted, skipped, failed,
		user.Username, time.Now().UTC().Format("2006-01-02 15:04:05"))

	menu := &telebot.ReplyMarkup{}
	data := fmt.Sprintf("%s|%s", nodeName, clusterName)
	btnDetail := menu.Data("🖥️ Node Details", "k8s_node_detail", data)
	btnList := menu.Data("◀️ All Nodes", "k8s_nodes_back", clusterName)
	menu.Inline(menu.Row(btnDetail, btnList))

	if c.Callback() != nil {
		_, _ = c.Bot().Edit(c.Callback().Message, finalText, menu, telebot.ModeMarkdown)
	}
}

// updateDrainMessage updates the drain progress message.
func (m *Module) updateDrainMessage(c telebot.Context, text string) {
	if c.Callback() != nil {
		_, _ = c.Bot().Edit(c.Callback().Message, text, telebot.ModeMarkdown)
	}
}

// handleNodeTopPods shows top pods on a specific node.
func (m *Module) handleNodeTopPods(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	nodeName, clusterName := parts[0], parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metricsClient, err := m.cluster.MetricsClient(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to metrics.")
	}

	allPodMetrics, err := metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return c.Send("⚠️ Failed to get pod metrics.")
	}

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	// Filter pods on this node
	pods, _ := clientSet.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", nodeName).String(),
	})

	nodePodsMap := make(map[string]bool)
	if pods != nil {
		for _, pod := range pods.Items {
			nodePodsMap[pod.Namespace+"/"+pod.Name] = true
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *Top Pods on %s* (%s)\n", nodeName, clusterName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

	count := 0
	for _, pm := range allPodMetrics.Items {
		if !nodePodsMap[pm.Namespace+"/"+pm.Name] {
			continue
		}

		totalCPU := int64(0)
		totalRAM := int64(0)
		for _, container := range pm.Containers {
			totalCPU += container.Usage.Cpu().MilliValue()
			totalRAM += container.Usage.Memory().Value()
		}

		sb.WriteString(fmt.Sprintf("📦 `%s`  CPU: %dm  RAM: %s\n", pm.Name, totalCPU, formatBytes(totalRAM)))
		count++
		if count >= 15 {
			sb.WriteString(fmt.Sprintf("\n...and %d more", len(allPodMetrics.Items)-15))
			break
		}
	}

	if count == 0 {
		sb.WriteString("No pod metrics found.")
	}

	menu := &telebot.ReplyMarkup{}
	data := fmt.Sprintf("%s|%s", nodeName, clusterName)
	btnBack := menu.Data("◀️ Back", "k8s_node_detail", data)
	menu.Inline(menu.Row(btnBack))

	_, err = c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
	return err
}

// handleNodesBack goes back to node list.
func (m *Module) handleNodesBack(c telebot.Context) error {
	return m.sendNodeList(c)
}

// nodeConditionDisplay returns status emoji and text for a node.
func nodeConditionDisplay(node *corev1.Node) (string, string) {
	if node.Spec.Unschedulable {
		return "🟡", "SchedulingDisabled"
	}

	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			switch cond.Status {
			case corev1.ConditionTrue:
				return "🟢", "Ready"
			case corev1.ConditionFalse:
				return "🔴", "NotReady"
			default:
				return "🔴", "Unknown"
			}
		}
	}
	return "⚪", "Unknown"
}

// isDaemonSetPod checks if a pod is managed by a DaemonSet.
func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}
