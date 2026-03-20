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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// handlePods handles the /pods command — shows namespace selector.
func (m *Module) handlePods(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesPodsList)
	if !allowed {
		return c.Send("⛔ You don't have permission to list pods.")
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

	markup := m.kb.NamespaceSelector(namespaces, "k8s")
	return c.Send(fmt.Sprintf("📦 Select namespace (%s):", clusterName), markup)
}

// handleNamespaceSelect handles namespace selection callback.
func (m *Module) handleNamespaceSelect(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	namespace := c.Callback().Data
	clusterName := m.userCtx.GetCluster(user.TelegramID)

	return m.sendPodList(c, clusterName, namespace, 1)
}

// sendPodList fetches and displays pods for a namespace.
func (m *Module) sendPodList(c telebot.Context, clusterName, namespace string, page int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	opts := metav1.ListOptions{}
	var podList *corev1.PodList

	if namespace == "_all" {
		podList, err = clientSet.CoreV1().Pods("").List(ctx, opts)
	} else {
		podList, err = clientSet.CoreV1().Pods(namespace).List(ctx, opts)
	}
	if err != nil {
		m.logger.Error("failed to list pods",
			zap.String("cluster", clusterName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to list pods.")
	}

	if len(podList.Items) == 0 {
		nsLabel := namespace
		if namespace == "_all" {
			nsLabel = "all namespaces"
		}
		return c.Send(fmt.Sprintf("📦 No pods found in %s (%s)", nsLabel, clusterName))
	}

	// Build pod lines
	var podLines []string
	for _, pod := range podList.Items {
		podLines = append(podLines, formatPodLine(&pod))
	}

	// Paginate
	pageSize := 8
	paginator := tfmt.NewPaginator(podLines, pageSize)
	paginator.Current = page

	items, _ := paginator.Page()

	nsLabel := namespace
	if namespace == "_all" {
		nsLabel = "all namespaces"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 *Pods in %s* (cluster: %s)\n\n", nsLabel, clusterName))
	for _, item := range items {
		sb.WriteString(item + "\n")
	}

	// Build keyboard with pagination + actions
	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	// Pod buttons (clickable)
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(podList.Items) {
		end = len(podList.Items)
	}
	for _, pod := range podList.Items[start:end] {
		podData := m.kb.StoreData(fmt.Sprintf("%s|%s|%s", pod.Name, pod.Namespace, clusterName))
		// Truncate long pod names: keep tail (hash suffix is the unique part)
		displayName := pod.Name
		const maxPodBtnText = 45
		if len(displayName) > maxPodBtnText {
			displayName = "…" + displayName[len(displayName)-(maxPodBtnText-1):]
		}
		btn := menu.Data(
			fmt.Sprintf("%s %s", tfmt.StatusEmoji(podStatus(&pod)), displayName),
			"k8s_pod_detail",
			podData,
		)
		rows = append(rows, menu.Row(btn))
	}

	// Action buttons
	var actionBtns []telebot.Btn
	actionBtns = append(actionBtns, menu.Data("🔄 Refresh", "k8s_pods_refresh", m.kb.StoreData(fmt.Sprintf("%s|%s|%d", namespace, clusterName, page))))
	if paginator.Current > 1 {
		actionBtns = append(actionBtns, menu.Data("◀️ Prev", "k8s_pods_page", m.kb.StoreData(fmt.Sprintf("%s|%s|%d", namespace, clusterName, page-1))))
	}
	if paginator.Current < paginator.TotalPages() {
		actionBtns = append(actionBtns, menu.Data("▶️ Next", "k8s_pods_page", m.kb.StoreData(fmt.Sprintf("%s|%s|%d", namespace, clusterName, page+1))))
	}
	rows = append(rows, menu.Row(actionBtns...))

	// Page info
	rows = append(rows, menu.Row(
		menu.Data(fmt.Sprintf("📄 Page %d/%d (%d pods)", page, paginator.TotalPages(), len(podList.Items)), "k8s_pods_info"),
	))

	menu.Inline(rows...)

	if c.Callback() != nil {
		_, err := c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
		return err
	}
	return c.Send(sb.String(), menu, telebot.ModeMarkdown)
}

// handlePodsPage handles pagination.
func (m *Module) handlePodsPage(c telebot.Context) error {
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

	return m.sendPodList(c, clusterName, namespace, page)
}

// handlePodsRefresh refreshes the pod list.
func (m *Module) handlePodsRefresh(c telebot.Context) error {
	return m.handlePodsPage(c) // Same logic
}

// handlePodsBack goes back to namespace selection.
func (m *Module) handlePodsBack(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	parts := strings.SplitN(c.Callback().Data, "|", 2)
	clusterName := m.userCtx.GetCluster(user.TelegramID)
	if len(parts) >= 2 {
		clusterName = parts[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	namespaces, err := m.getNamespaces(ctx, clusterName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Failed to list namespaces"})
	}

	markup := m.kb.NamespaceSelector(namespaces, "k8s")
	_, editErr := c.Bot().Edit(c.Callback().Message, fmt.Sprintf("📦 Select namespace (%s):", clusterName), markup)
	return editErr
}

// handlePodDetail shows detailed information for a specific pod.
func (m *Module) handlePodDetail(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) != 3 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	podName, namespace, clusterName := parts[0], parts[1], parts[2]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Cluster error"})
	}

	pod, err := clientSet.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Pod not found"})
	}

	text := formatPodDetail(pod, clusterName)
	markup := m.kb.PodActions(podName, namespace, clusterName)

	if c.Callback() != nil {
		_, err := c.Bot().Edit(c.Callback().Message, text, markup, telebot.ModeMarkdown)
		return err
	}
	return c.Send(text, markup, telebot.ModeMarkdown)
}

// formatPodLine formats a single pod line for the list view.
func formatPodLine(pod *corev1.Pod) string {
	status := podStatus(pod)
	emoji := tfmt.StatusEmoji(status)
	restarts := int32(0)
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}

	age := time.Since(pod.CreationTimestamp.Time)
	ageStr := formatDuration(age)

	line := fmt.Sprintf("%s `%s` %s %s", emoji, pod.Name, status, ageStr)
	if restarts > 0 {
		line += fmt.Sprintf(" (restarts: %d)", restarts)
	}
	return line
}

// formatPodDetail formats detailed pod info.
func formatPodDetail(pod *corev1.Pod, clusterName string) string {
	status := podStatus(pod)
	emoji := tfmt.StatusEmoji(status)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 *%s*\n", pod.Name))
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("Namespace:  `%s`\n", pod.Namespace))
	sb.WriteString(fmt.Sprintf("Status:     %s %s\n", emoji, status))
	sb.WriteString(fmt.Sprintf("Cluster:    %s\n", clusterName))

	if pod.Spec.NodeName != "" {
		sb.WriteString(fmt.Sprintf("Node:       `%s`\n", pod.Spec.NodeName))
	}
	if pod.Status.PodIP != "" {
		sb.WriteString(fmt.Sprintf("IP:         `%s`\n", pod.Status.PodIP))
	}

	age := time.Since(pod.CreationTimestamp.Time)
	sb.WriteString(fmt.Sprintf("Age:        %s\n", formatDuration(age)))

	// Images
	var images []string
	for _, c := range pod.Spec.Containers {
		images = append(images, c.Image)
	}
	sb.WriteString(fmt.Sprintf("Images:     %s\n", strings.Join(images, ", ")))

	// Container statuses
	sb.WriteString("\n*Containers:*\n")
	for _, cs := range pod.Status.ContainerStatuses {
		cEmoji := "🟢"
		cStatus := "Running"
		if cs.State.Waiting != nil {
			cEmoji = "🔴"
			cStatus = cs.State.Waiting.Reason
		} else if cs.State.Terminated != nil {
			cEmoji = "⚪"
			cStatus = cs.State.Terminated.Reason
			if cStatus == "" {
				cStatus = fmt.Sprintf("exit code %d", cs.State.Terminated.ExitCode)
			}
		}
		sb.WriteString(fmt.Sprintf("  %s `%s` — %s", cEmoji, cs.Name, cStatus))
		if cs.RestartCount > 0 {
			sb.WriteString(fmt.Sprintf(" (restarts: %d)", cs.RestartCount))
		}
		sb.WriteString("\n")
	}

	// Init containers
	for _, cs := range pod.Status.InitContainerStatuses {
		cEmoji := "🟡"
		cStatus := "Init"
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode == 0 {
			cEmoji = "⚪"
			cStatus = "Completed"
		} else if cs.State.Waiting != nil {
			cStatus = cs.State.Waiting.Reason
		}
		sb.WriteString(fmt.Sprintf("  %s `%s` (init) — %s\n", cEmoji, cs.Name, cStatus))
	}

	return sb.String()
}

// podStatus returns the effective status string for a pod.
func podStatus(pod *corev1.Pod) string {
	// Check container statuses for more specific states
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
	}

	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}

	return string(pod.Status.Phase)
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
