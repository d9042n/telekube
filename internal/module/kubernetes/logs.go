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

// handleLogsCommand handles the /logs <pod> command.
func (m *Module) handleLogsCommand(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesPodsLogs)
	if !allowed {
		return c.Send("⛔ You don't have permission to view logs.")
	}

	args := strings.Fields(c.Text())
	if len(args) < 2 {
		return c.Send("Usage: /logs <pod-name> [namespace]\nOr use the 📋 Logs button from pod detail view.")
	}

	podName := args[1]
	clusterName := m.userCtx.GetCluster(user.TelegramID)

	// Determine namespace
	namespace := "default"
	if len(args) >= 3 {
		namespace = args[2]
	}

	return m.fetchAndSendLogs(c, podName, namespace, clusterName, "", 50, false)
}

// handleLogs handles the Logs button callback.
func (m *Module) handleLogs(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) != 3 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	podName, namespace, clusterName := parts[0], parts[1], parts[2]

	// Check if pod has multiple containers — show container selector
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

	if len(pod.Spec.Containers) > 1 {
		// Show container selector
		menu := &telebot.ReplyMarkup{}
		var rows []telebot.Row
		for _, container := range pod.Spec.Containers {
			data := m.kb.StoreData(fmt.Sprintf("%s|%s|%s|%s", podName, namespace, clusterName, container.Name))
			btn := menu.Data(container.Name, "k8s_logs_container", data)
			rows = append(rows, menu.Row(btn))
		}
		btnBack := menu.Data("◀️ Back", "k8s_pod_detail", m.kb.StoreData(fmt.Sprintf("%s|%s|%s", podName, namespace, clusterName)))
		rows = append(rows, menu.Row(btnBack))
		menu.Inline(rows...)

		_, err := c.Bot().Edit(c.Callback().Message, "Select container:", menu)
		return err
	}

	// Single container
	return m.fetchAndSendLogs(c, podName, namespace, clusterName, pod.Spec.Containers[0].Name, 50, false)
}

// handleLogsContainer handles container selection callback.
func (m *Module) handleLogsContainer(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 4)
	if len(parts) != 4 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	podName, namespace, clusterName, container := parts[0], parts[1], parts[2], parts[3]
	return m.fetchAndSendLogs(c, podName, namespace, clusterName, container, 50, false)
}

// handleLogsMore handles "more lines" callback.
func (m *Module) handleLogsMore(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 5)
	if len(parts) != 5 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	podName, namespace, clusterName, container := parts[0], parts[1], parts[2], parts[3]
	var tailLines int
	if _, err := fmt.Sscanf(parts[4], "%d", &tailLines); err != nil {
		tailLines = 100
	}
	if tailLines <= 0 {
		tailLines = 100
	}

	return m.fetchAndSendLogs(c, podName, namespace, clusterName, container, int64(tailLines), false)
}

// handleLogsPrevious handles "previous container" logs.
func (m *Module) handleLogsPrevious(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 4)
	if len(parts) != 4 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	podName, namespace, clusterName, container := parts[0], parts[1], parts[2], parts[3]
	return m.fetchAndSendLogs(c, podName, namespace, clusterName, container, 50, true)
}

// fetchAndSendLogs fetches pod logs and sends them.
func (m *Module) fetchAndSendLogs(c telebot.Context, podName, namespace, clusterName, container string, tailLines int64, previous bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	opts := &corev1.PodLogOptions{
		TailLines: &tailLines,
		Previous:  previous,
	}
	if container != "" {
		opts.Container = container
	}

	req := clientSet.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		m.logger.Error("failed to get pod logs",
			zap.String("pod", podName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		errMsg := "⚠️ Failed to get logs."
		if previous {
			errMsg = "⚠️ No previous container logs available."
		}
		if c.Callback() != nil {
			return c.Respond(&telebot.CallbackResponse{Text: errMsg})
		}
		return c.Send(errMsg)
	}
	defer func() { _ = stream.Close() }()

	// Read logs
	buf := make([]byte, 4096*4)
	n, _ := stream.Read(buf)
	logText := string(buf[:n])

	if logText == "" {
		logText = "(no logs)"
	}

	// Build header
	containerLabel := container
	if containerLabel == "" {
		containerLabel = "default"
	}
	prevLabel := ""
	if previous {
		prevLabel = " (previous)"
	}

	header := fmt.Sprintf("📋 *Logs: %s/%s*%s (last %d lines)\n━━━━━━━━━━━━━━━━━━\n",
		podName, containerLabel, prevLabel, tailLines)

	// Split and send messages if too long
	fullText := header + tfmt.CodeBlock(logText)
	parts := tfmt.SplitMessage(fullText, 4000)

	markup := m.kb.LogActions(podName, namespace, clusterName, container, int(tailLines))

	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part gets the keyboard
			if c.Callback() != nil && i == 0 {
				_, err := c.Bot().Edit(c.Callback().Message, part, markup, telebot.ModeMarkdown)
				return err
			}
			return c.Send(part, markup, telebot.ModeMarkdown)
		}
		// Non-last parts
		_ = c.Send(part, telebot.ModeMarkdown)
	}

	return nil
}
