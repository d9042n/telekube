package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/bot/middleware"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// handleRestartCommand handles the /restart <pod> [namespace] slash command.
func (m *Module) handleRestartCommand(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesPodsRestart)
	if !allowed {
		m.audit.Log(entity.AuditEntry{
			ID:         ulid.Make().String(),
			UserID:     user.TelegramID,
			Username:   user.Username,
			Action:     "pod.restart",
			Status:     entity.AuditStatusDenied,
			OccurredAt: time.Now().UTC(),
		})
		return c.Send("⛔ You don't have permission to restart pods.")
	}

	args := strings.Fields(c.Text())
	if len(args) < 2 {
		return c.Send("Usage: `/restart <pod-name> [namespace]`", telebot.ModeMarkdown)
	}

	podName := args[1]
	clusterName := m.userCtx.GetCluster(user.TelegramID)
	if clusterName == "" {
		return c.Send("⚠️ No cluster selected. Use /clusters to select one.")
	}

	namespace := "default"
	if len(args) >= 3 {
		namespace = args[2]
	}

	// Verify pod exists and has an owner
	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	pod, err := clientSet.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		m.logger.Error("pod not found for restart command",
			zap.String("pod", podName),
			zap.String("namespace", namespace),
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return c.Send(fmt.Sprintf("⚠️ Pod `%s` not found in namespace `%s`.", podName, namespace), telebot.ModeMarkdown)
	}

	if len(pod.OwnerReferences) == 0 {
		return c.Send("⛔ Cannot restart standalone pods (no controller). Use delete instead.")
	}

	// Show confirmation
	text := fmt.Sprintf("⚠️ *Confirm restart pod %s?*\n\nCluster: `%s`\nNamespace: `%s`\n\nThis will delete the pod. The controller will recreate it.",
		podName, clusterName, namespace)

	menu := &telebot.ReplyMarkup{}
	data := fmt.Sprintf("%s|%s|%s", podName, namespace, clusterName)
	btnConfirm := menu.Data("✅ Confirm", "k8s_restart_confirm", data)
	btnCancel := menu.Data("❌ Cancel", "k8s_restart_cancel", data)
	menu.Inline(menu.Row(btnConfirm, btnCancel))

	return c.Send(text, menu, telebot.ModeMarkdown)
}
