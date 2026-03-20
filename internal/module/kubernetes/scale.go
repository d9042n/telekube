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
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	scaleMaxDefault = 50
	scaleTimeout    = 120 * time.Second
)

// displayName returns the user's @username or a fallback like "user_12345".
func displayName(user *entity.User) string {
	if user == nil {
		return "unknown"
	}
	if user.Username != "" {
		return "@" + user.Username
	}
	if user.DisplayName != "" {
		return user.DisplayName
	}
	return fmt.Sprintf("user_%d", user.TelegramID)
}

// handleScale handles the /scale command — shows namespace selector for scaling.
func (m *Module) handleScale(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesDeploymentsScale)
	if !allowed {
		return c.Send("⛔ You don't have permission to scale resources.")
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

	markup := m.kb.NamespaceSelector(namespaces, "k8s_scale")
	return c.Send(fmt.Sprintf("📦 Select namespace to scale resources (%s):", clusterName), markup)
}

// handleScaleNamespaceSelect handles namespace selection for scaling.
func (m *Module) handleScaleNamespaceSelect(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	namespace := c.Callback().Data
	clusterName := m.userCtx.GetCluster(user.TelegramID)

	return m.sendScaleableResources(c, clusterName, namespace)
}

// sendScaleableResources lists deployments and statefulsets in a namespace.
func (m *Module) sendScaleableResources(c telebot.Context, clusterName, namespace string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Send("⚠️ Failed to connect to cluster.")
	}

	var deployments *appsv1.DeploymentList
	var statefulsets *appsv1.StatefulSetList

	if namespace == "_all" {
		deployments, err = clientSet.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	} else {
		deployments, err = clientSet.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		m.logger.Error("failed to list deployments", zap.Error(err))
		return c.Send("⚠️ Failed to list deployments.")
	}

	if namespace == "_all" {
		statefulsets, err = clientSet.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	} else {
		statefulsets, err = clientSet.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		m.logger.Error("failed to list statefulsets", zap.Error(err))
		return c.Send("⚠️ Failed to list statefulsets.")
	}

	if len(deployments.Items) == 0 && len(statefulsets.Items) == 0 {
		nsLabel := namespace
		if namespace == "_all" {
			nsLabel = "all namespaces"
		}
		return c.Send(fmt.Sprintf("📦 No scaleable resources in %s (%s)", nsLabel, clusterName))
	}

	nsLabel := namespace
	if namespace == "_all" {
		nsLabel = "all namespaces"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 *Scaleable Resources* — %s (%s)\n", nsLabel, clusterName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	if len(deployments.Items) > 0 {
		sb.WriteString("*Deployments:*\n")
		for _, d := range deployments.Items {
			replicas := int32(1)
			if d.Spec.Replicas != nil {
				replicas = *d.Spec.Replicas
			}
			sb.WriteString(fmt.Sprintf("  📦 `%s` — %d/%d ready\n", d.Name, d.Status.ReadyReplicas, replicas))

			data := m.sd(fmt.Sprintf("deploy|%s|%s|%s", d.Name, d.Namespace, clusterName))
			btn := menu.Data(
				fmt.Sprintf("📦 %s (%d)", d.Name, replicas),
				"k8s_scale_detail",
				data,
			)
			rows = append(rows, menu.Row(btn))
		}
	}

	if len(statefulsets.Items) > 0 {
		sb.WriteString("\n*StatefulSets:*\n")
		for _, s := range statefulsets.Items {
			replicas := int32(1)
			if s.Spec.Replicas != nil {
				replicas = *s.Spec.Replicas
			}
			sb.WriteString(fmt.Sprintf("  📦 `%s` — %d/%d ready\n", s.Name, s.Status.ReadyReplicas, replicas))

			data := m.sd(fmt.Sprintf("sts|%s|%s|%s", s.Name, s.Namespace, clusterName))
			btn := menu.Data(
				fmt.Sprintf("📦 %s (%d)", s.Name, replicas),
				"k8s_scale_detail",
				data,
			)
			rows = append(rows, menu.Row(btn))
		}
	}

	rows = append(rows, menu.Row(
		menu.Data("◀️ Back", "k8s_scale_back", clusterName),
	))

	menu.Inline(rows...)

	if c.Callback() != nil {
		_, err := c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
		if err != nil {
			m.logger.Warn("scale: markdown edit failed, retrying as plain text",
				zap.String("namespace", namespace),
				zap.Error(err),
			)
			// Fallback: strip Markdown markers and send as plain text
			plain := strings.ReplaceAll(sb.String(), "*", "")
			plain = strings.ReplaceAll(plain, "`", "")
			_, err = c.Bot().Edit(c.Callback().Message, plain, menu)
		}
		return err
	}
	err = c.Send(sb.String(), menu, telebot.ModeMarkdown)
	if err != nil {
		m.logger.Warn("scale: markdown send failed, retrying as plain text", zap.Error(err))
		plain := strings.ReplaceAll(sb.String(), "*", "")
		plain = strings.ReplaceAll(plain, "`", "")
		return c.Send(plain, menu)
	}
	return nil
}

// handleScaleDetail shows current replicas and scale options.
func (m *Module) handleScaleDetail(c telebot.Context) error {
	// Format: type|name|namespace|cluster
	parts := strings.SplitN(c.Callback().Data, "|", 4)
	if len(parts) != 4 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	kind, name, namespace, clusterName := parts[0], parts[1], parts[2], parts[3]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Cluster error"})
	}

	var currentReplicas int32
	var readyReplicas int32
	var available int32

	switch kind {
	case "deploy":
		deploy, err := clientSet.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Resource not found"})
		}
		if deploy.Spec.Replicas != nil {
			currentReplicas = *deploy.Spec.Replicas
		}
		readyReplicas = deploy.Status.ReadyReplicas
		available = deploy.Status.AvailableReplicas
	case "sts":
		sts, err := clientSet.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Resource not found"})
		}
		if sts.Spec.Replicas != nil {
			currentReplicas = *sts.Spec.Replicas
		}
		readyReplicas = sts.Status.ReadyReplicas
		available = readyReplicas
	default:
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Unknown resource type"})
	}

	kindLabel := "Deployment"
	if kind == "sts" {
		kindLabel = "StatefulSet"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📦 *%s* — %s\n", name, kindLabel))
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("Current:   %d replicas\n", currentReplicas))
	sb.WriteString(fmt.Sprintf("Ready:     %d/%d\n", readyReplicas, currentReplicas))
	sb.WriteString(fmt.Sprintf("Available: %d\n", available))
	sb.WriteString(fmt.Sprintf("Cluster:   %s\n", clusterName))
	sb.WriteString(fmt.Sprintf("Namespace: %s\n\n", namespace))
	sb.WriteString("*Set replicas:*\n")

	menu := &telebot.ReplyMarkup{}
	baseData := fmt.Sprintf("%s|%s|%s|%s", kind, name, namespace, clusterName)

	// Quick scale buttons
	var quickBtns []telebot.Btn
	// -1 button
	if currentReplicas > 0 {
		quickBtns = append(quickBtns, menu.Data("-1", "k8s_scale_set",
			m.sd(fmt.Sprintf("%s|%d", baseData, currentReplicas-1))))
	}
	// Preset values
	for _, n := range []int32{1, 2, 3, 5, 10} {
		if n != currentReplicas {
			quickBtns = append(quickBtns, menu.Data(fmt.Sprintf("%d", n), "k8s_scale_set",
				m.sd(fmt.Sprintf("%s|%d", baseData, n))))
		}
	}
	// +1 button
	quickBtns = append(quickBtns, menu.Data("+1", "k8s_scale_set",
		m.sd(fmt.Sprintf("%s|%d", baseData, currentReplicas+1))))

	// Arrange buttons in rows of 4
	var rows []telebot.Row
	for i := 0; i < len(quickBtns); i += 4 {
		end := i + 4
		if end > len(quickBtns) {
			end = len(quickBtns)
		}
		rows = append(rows, menu.Row(quickBtns[i:end]...))
	}

	rows = append(rows, menu.Row(
		menu.Data("◀️ Back", "k8s_scale_ns", m.sd(fmt.Sprintf("%s|%s", namespace, clusterName))),
	))

	menu.Inline(rows...)

	_, err = c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
	if err != nil {
		m.logger.Warn("scale detail: markdown edit failed, retrying as plain text",
			zap.String("name", name),
			zap.Error(err),
		)
		plain := strings.ReplaceAll(sb.String(), "*", "")
		plain = strings.ReplaceAll(plain, "`", "")
		_, err = c.Bot().Edit(c.Callback().Message, plain, menu)
	}
	return err
}

// handleScaleSet shows confirmation before scaling.
func (m *Module) handleScaleSet(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	// Format: type|name|namespace|cluster|targetReplicas
	parts := strings.SplitN(c.Callback().Data, "|", 5)
	if len(parts) != 5 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	kind, name, namespace, clusterName := parts[0], parts[1], parts[2], parts[3]
	var targetReplicas int32
	fmt.Sscanf(parts[4], "%d", &targetReplicas)

	if targetReplicas < 0 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Cannot scale to negative replicas"})
	}
	if targetReplicas > scaleMaxDefault {
		return c.Respond(&telebot.CallbackResponse{
			Text:      fmt.Sprintf("⛔ Cannot scale above %d replicas (safety limit)", scaleMaxDefault),
			ShowAlert: true,
		})
	}

	// Get current replicas
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Cluster error"})
	}

	var currentReplicas int32
	switch kind {
	case "deploy":
		deploy, err := clientSet.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Resource not found"})
		}
		if deploy.Spec.Replicas != nil {
			currentReplicas = *deploy.Spec.Replicas
		}
	case "sts":
		sts, err := clientSet.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Resource not found"})
		}
		if sts.Spec.Replicas != nil {
			currentReplicas = *sts.Spec.Replicas
		}
	}

	// Special confirmation for scaling to 0
	extraWarning := ""
	if targetReplicas == 0 {
		extraWarning = "\n\n🚨 *Scaling to 0 will stop all instances!*"
	}

	kindLabel := "Deployment"
	if kind == "sts" {
		kindLabel = "StatefulSet"
	}

	text := fmt.Sprintf("⚠️ *Scale %s %s from %d → %d replicas?*\n\nCluster: %s\nNamespace: %s%s",
		kindLabel, name, currentReplicas, targetReplicas, clusterName, namespace, extraWarning)

	menu := &telebot.ReplyMarkup{}
	btnConfirm := menu.Data("✅ Confirm", "k8s_scale_confirm", m.sd(c.Callback().Data))
	btnCancel := menu.Data("❌ Cancel", "k8s_scale_cancel",
		m.sd(fmt.Sprintf("%s|%s|%s|%s", kind, name, namespace, clusterName)))
	menu.Inline(menu.Row(btnConfirm, btnCancel))

	_, err = c.Bot().Edit(c.Callback().Message, text, menu, telebot.ModeMarkdown)
	return err
}

// handleScaleConfirm executes the scale operation.
func (m *Module) handleScaleConfirm(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	parts := strings.SplitN(c.Callback().Data, "|", 5)
	if len(parts) != 5 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	kind, name, namespace, clusterName := parts[0], parts[1], parts[2], parts[3]
	var targetReplicas int32
	fmt.Sscanf(parts[4], "%d", &targetReplicas)

	// Check approval requirement
	if m.approval != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		needsApproval, err := m.approval.CheckAndSubmit(ctx, c, approval.ApprovalInput{
			UserID:       user.TelegramID,
			Username:     user.Username,
			Action:       "kubernetes.deployments.scale",
			Resource:     name,
			Cluster:      clusterName,
			Namespace:    namespace,
			Details:      map[string]interface{}{"kind": kind, "target_replicas": targetReplicas},
			CallbackData: c.Callback().Data,
		})
		if err != nil {
			m.logger.Error("approval check failed", zap.Error(err))
		}
		if needsApproval {
			return c.Respond(&telebot.CallbackResponse{
				Text: fmt.Sprintf("📋 Scaling %s requires approval. Request submitted.", name),
			})
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), scaleTimeout)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Cluster error"})
	}

	kindLabel := "Deployment"
	if kind == "sts" {
		kindLabel = "StatefulSet"
	}

	// Perform the scale operation
	scale := &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: autoscalingv1.ScaleSpec{
			Replicas: targetReplicas,
		},
	}

	switch kind {
	case "deploy":
		_, err = clientSet.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	case "sts":
		_, err = clientSet.AppsV1().StatefulSets(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	}

	if err != nil {
		m.logger.Error("failed to scale resource",
			zap.String("kind", kindLabel),
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.String("cluster", clusterName),
			zap.Error(err),
		)

		m.audit.Log(entity.AuditEntry{
			ID:         ulid.Make().String(),
			UserID:     user.TelegramID,
			Username:   user.Username,
			Action:     fmt.Sprintf("%s.scale", kind),
			Resource:   fmt.Sprintf("%s/%s", kindLabel, name),
			Cluster:    clusterName,
			Namespace:  namespace,
			Status:     entity.AuditStatusError,
			Error:      err.Error(),
			Details:    map[string]interface{}{"target_replicas": targetReplicas},
			OccurredAt: time.Now().UTC(),
		})

		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Failed to scale"})
	}

	// Audit success
	m.audit.Log(entity.AuditEntry{
		ID:         ulid.Make().String(),
		UserID:     user.TelegramID,
		Username:   user.Username,
		Action:     fmt.Sprintf("%s.scale", kind),
		Resource:   fmt.Sprintf("%s/%s", kindLabel, name),
		Cluster:    clusterName,
		Namespace:  namespace,
		Status:     entity.AuditStatusSuccess,
		Details:    map[string]interface{}{"target_replicas": targetReplicas},
		OccurredAt: time.Now().UTC(),
	})

	// Show initial success with progress
	text := fmt.Sprintf("✅ *Scaled %s %s to %d replicas*\n\n🔄 Waiting for rollout...\n\nTriggered by: %s at %s",
		kindLabel, name, targetReplicas,
		displayName(user), time.Now().UTC().Format("2006-01-02 15:04:05"))

	_, editErr := c.Bot().Edit(c.Callback().Message, text, telebot.ModeMarkdown)
	if editErr != nil {
		m.logger.Error("failed to edit scale message", zap.Error(editErr))
	}

	// Watch rollout in background (update message with progress)
	go m.watchRollout(c, kind, name, namespace, clusterName, targetReplicas)

	return nil
}

// watchRollout monitors the rollout progress and updates the message.
func (m *Module) watchRollout(c telebot.Context, kind, name, namespace, clusterName string, target int32) {
	ctx, cancel := context.WithTimeout(context.Background(), scaleTimeout)
	defer cancel()

	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return
	}

	kindLabel := "Deployment"
	if kind == "sts" {
		kindLabel = "StatefulSet"
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var ready int32

			switch kind {
			case "deploy":
				deploy, err := clientSet.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
				if err != nil {
					return
				}
				ready = deploy.Status.ReadyReplicas
			case "sts":
				sts, err := clientSet.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
				if err != nil {
					return
				}
				ready = sts.Status.ReadyReplicas
			}

			user := middleware.GetUser(c)
			uname := displayName(user)

			if ready >= target {
				text := fmt.Sprintf("✅ *Scaled %s %s to %d replicas*\n\n✅ Rollout complete: %d/%d ready\n\nTriggered by: %s",
					kindLabel, name, target, ready, target, uname)

				if c.Callback() != nil {
					_, _ = c.Bot().Edit(c.Callback().Message, text, telebot.ModeMarkdown)
				}
				return
			}

			text := fmt.Sprintf("✅ *Scaled %s %s to %d replicas*\n\n🔄 Progress: %d/%d ready...\n\nTriggered by: %s",
				kindLabel, name, target, ready, target, uname)

			if c.Callback() != nil {
				_, _ = c.Bot().Edit(c.Callback().Message, text, telebot.ModeMarkdown)
			}
		}
	}
}

// handleScaleCancel cancels the scale operation and goes back to detail.
func (m *Module) handleScaleCancel(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 4)
	if len(parts) != 4 {
		return c.Respond(&telebot.CallbackResponse{Text: "Cancelled"})
	}

	// Redirect to detail view
	c.Callback().Data = strings.Join(parts, "|")
	return m.handleScaleDetail(c)
}

// handleScaleBack goes back to namespace selection for /scale.
func (m *Module) handleScaleBack(c telebot.Context) error {
	return m.handleScale(c)
}

// handleScaleNsBack goes back to resource list for a namespace.
func (m *Module) handleScaleNsBack(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	namespace, clusterName := parts[0], parts[1]
	return m.sendScaleableResources(c, clusterName, namespace)
}
