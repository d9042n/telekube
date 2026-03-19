package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/rbac"
	tfmt "github.com/d9042n/telekube/pkg/telegram"
	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// handleEventsCommand handles the /events <pod> command.
func (m *Module) handleEventsCommand(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesPodsEvents)
	if !allowed {
		return c.Send("⛔ You don't have permission to view events.")
	}

	args := strings.Fields(c.Text())
	if len(args) < 2 {
		return c.Send("Usage: /events <pod-name> [namespace]")
	}

	podName := args[1]
	clusterName := m.userCtx.GetCluster(user.TelegramID)
	namespace := "default"
	if len(args) >= 3 {
		namespace = args[2]
	}

	return m.fetchAndSendEvents(c, podName, namespace, clusterName)
}

// handleEvents handles the Events button callback.
func (m *Module) handleEvents(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) != 3 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}
	return m.fetchAndSendEvents(c, parts[0], parts[1], parts[2])
}

// handleEventsRefresh refreshes events.
func (m *Module) handleEventsRefresh(c telebot.Context) error {
	return m.handleEvents(c)
}

// fetchAndSendEvents fetches events for a pod and sends them.
func (m *Module) fetchAndSendEvents(c telebot.Context, podName, namespace, clusterName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	// Get events related to this pod
	fieldSelector := fmt.Sprintf("involvedObject.name=%s,involvedObject.namespace=%s", podName, namespace)
	events, err := clientSet.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		m.logger.Error("failed to list events",
			zap.String("pod", podName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to get events.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 *Events for %s*\n", podName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

	if len(events.Items) == 0 {
		sb.WriteString("No events found.")
	} else {
		for _, event := range events.Items {
			emoji := tfmt.EventEmoji(event.Type)
			eventTime := event.LastTimestamp.Format("15:04:05")
			if event.LastTimestamp.IsZero() {
				eventTime = event.CreationTimestamp.Format("15:04:05")
			}

			sb.WriteString(fmt.Sprintf("%s %s — *%s*: %s\n",
				emoji, eventTime, event.Reason, event.Message))

			if event.Count > 1 {
				sb.WriteString(fmt.Sprintf("   (occurred %d times)\n", event.Count))
			}
		}
	}

	// Buttons
	menu := &telebot.ReplyMarkup{}
	data := fmt.Sprintf("%s|%s|%s", podName, namespace, clusterName)
	btnRefresh := menu.Data("🔄 Refresh", "k8s_events_refresh", data)
	btnBack := menu.Data("◀️ Back", "k8s_pod_detail", data)
	menu.Inline(menu.Row(btnRefresh, btnBack))

	if c.Callback() != nil {
		_, err := c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
		return err
	}
	return c.Send(sb.String(), menu, telebot.ModeMarkdown)
}

// handleRestart shows restart confirmation dialog.
func (m *Module) handleRestart(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesPodsRestart)
	if !allowed {
		// Log denied action
		m.audit.Log(entity.AuditEntry{
			ID:         ulid.Make().String(),
			UserID:     user.TelegramID,
			Username:   user.Username,
			Action:     "pod.restart",
			Resource:   c.Callback().Data,
			Status:     entity.AuditStatusDenied,
			OccurredAt: time.Now().UTC(),
		})
		return c.Respond(&telebot.CallbackResponse{Text: "⛔ You don't have permission to restart pods."})
	}

	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) != 3 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	podName, namespace, clusterName := parts[0], parts[1], parts[2]

	text := fmt.Sprintf("⚠️ *Confirm restart pod %s?*\n\nCluster: %s\nNamespace: %s\n\nThis will delete the pod. The controller will recreate it.",
		podName, clusterName, namespace)
	markup := m.kb.Confirmation("k8s_restart", c.Callback().Data)

	_, err := c.Bot().Edit(c.Callback().Message, text, markup, telebot.ModeMarkdown)
	return err
}

// handleRestartConfirm executes the pod restart.
func (m *Module) handleRestartConfirm(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) != 3 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	podName, namespace, clusterName := parts[0], parts[1], parts[2]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify pod has an owner (don't delete standalone pods)
	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Cluster error"})
	}

	pod, err := clientSet.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Pod not found"})
	}

	if len(pod.OwnerReferences) == 0 {
		return c.Respond(&telebot.CallbackResponse{
			Text:      "⛔ Cannot restart standalone pods (no controller). Use delete instead.",
			ShowAlert: true,
		})
	}

	// Delete the pod
	deletePolicy := metav1.DeletePropagationForeground
	err = clientSet.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})
	if err != nil {
		m.logger.Error("failed to restart pod",
			zap.String("pod", podName),
			zap.String("namespace", namespace),
			zap.String("cluster", clusterName),
			zap.Error(err),
		)

		m.audit.Log(entity.AuditEntry{
			ID:         ulid.Make().String(),
			UserID:     user.TelegramID,
			Username:   user.Username,
			Action:     "pod.restart",
			Resource:   fmt.Sprintf("pod/%s", podName),
			Cluster:    clusterName,
			Namespace:  namespace,
			Status:     entity.AuditStatusError,
			Error:      err.Error(),
			OccurredAt: time.Now().UTC(),
		})

		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Failed to restart pod"})
	}

	// Audit success
	m.audit.Log(entity.AuditEntry{
		ID:         ulid.Make().String(),
		UserID:     user.TelegramID,
		Username:   user.Username,
		Action:     "pod.restart",
		Resource:   fmt.Sprintf("pod/%s", podName),
		Cluster:    clusterName,
		Namespace:  namespace,
		Status:     entity.AuditStatusSuccess,
		OccurredAt: time.Now().UTC(),
	})

	text := fmt.Sprintf("✅ Pod `%s` deleted (will be recreated by controller)\n\nTriggered by: @%s at %s",
		podName, user.Username, time.Now().UTC().Format("2006-01-02 15:04:05"))

	_, err = c.Bot().Edit(c.Callback().Message, text, telebot.ModeMarkdown)
	return err
}

// handleRestartCancel cancels the restart.
func (m *Module) handleRestartCancel(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 3)
	if len(parts) != 3 {
		return c.Respond(&telebot.CallbackResponse{Text: "Cancelled"})
	}

	podName, namespace, clusterName := parts[0], parts[1], parts[2]

	// Go back to pod detail
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

	_, editErr := c.Bot().Edit(c.Callback().Message, text, markup, telebot.ModeMarkdown)
	return editErr
}
