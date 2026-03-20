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

// handleCronJobsCommand handles the /cronjobs [namespace] slash command.
func (m *Module) handleCronJobsCommand(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Send("⚠️ Could not identify you.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	allowed, _ := m.rbac.HasPermission(ctx, user.TelegramID, rbac.PermKubernetesCronJobsList)
	if !allowed {
		return c.Send("⛔ You don't have permission to list CronJobs.")
	}

	clusterName := m.userCtx.GetCluster(user.TelegramID)
	if clusterName == "" {
		return c.Send("⚠️ No cluster selected. Use /clusters to select one.")
	}

	// Check if namespace provided as argument
	args := strings.Fields(c.Text())
	if len(args) >= 2 {
		namespace := args[1]
		return m.sendCronJobList(c, clusterName, namespace)
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

	markup := m.kb.NamespaceSelector(namespaces, "k8s_cronjobs")
	return c.Send(fmt.Sprintf("⏰ Select namespace for CronJobs (%s):", clusterName), markup)
}

// handleCronJobsNamespaceSelect handles namespace selection for cronjobs.
func (m *Module) handleCronJobsNamespaceSelect(c telebot.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Error"})
	}

	namespace := c.Callback().Data
	clusterName := m.userCtx.GetCluster(user.TelegramID)

	return m.sendCronJobList(c, clusterName, namespace)
}

// sendCronJobList fetches and displays CronJobs for a namespace.
func (m *Module) sendCronJobList(c telebot.Context, clusterName, namespace string) error {
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

	cronJobs, err := clientSet.BatchV1().CronJobs(listNs).List(ctx, metav1.ListOptions{})
	if err != nil {
		m.logger.Error("failed to list cronjobs",
			zap.String("cluster", clusterName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return c.Send("⚠️ Failed to list CronJobs.")
	}

	if len(cronJobs.Items) == 0 {
		return c.Send(fmt.Sprintf("⏰ No CronJobs found in %s (%s)", nsLabel, clusterName))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "⏰ *CronJobs in %s* (cluster: %s)\n", nsLabel, clusterName)
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

	for _, cj := range cronJobs.Items {
		// Determine status
		emoji := "🟢"
		status := "Active"
		if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
			emoji = "⏸️"
			status = "Suspended"
		}

		nsStr := ""
		if namespace == "_all" {
			nsStr = fmt.Sprintf(" [%s]", cj.Namespace)
		}

		fmt.Fprintf(&sb, "%s `%s`%s — %s\n", emoji, cj.Name, nsStr, status)
		fmt.Fprintf(&sb, "   Schedule: `%s`\n", cj.Spec.Schedule)

		// Last schedule time
		if cj.Status.LastScheduleTime != nil {
			lastRun := cj.Status.LastScheduleTime.Time
			ago := time.Since(lastRun)
			fmt.Fprintf(&sb, "   Last Run: %s (%s ago)\n", lastRun.Format("2006-01-02 15:04:05"), formatDuration(ago))
		} else {
			sb.WriteString("   Last Run: Never\n")
		}

		// Last successful time
		if cj.Status.LastSuccessfulTime != nil {
			fmt.Fprintf(&sb, "   Last Success: %s\n", cj.Status.LastSuccessfulTime.Format("2006-01-02 15:04:05"))
		}

		// Active jobs count
		activeJobs := len(cj.Status.Active)
		if activeJobs > 0 {
			fmt.Fprintf(&sb, "   🔄 Active Jobs: %d\n", activeJobs)
		}

		sb.WriteString("\n")
	}

	// Build keyboard
	menu := &telebot.ReplyMarkup{}
	data := m.sd(fmt.Sprintf("%s|%s", namespace, clusterName))
	btnRefresh := menu.Data("🔄 Refresh", "k8s_cronjobs_refresh", data)
	menu.Inline(menu.Row(btnRefresh))

	if c.Callback() != nil {
		_, err := c.Bot().Edit(c.Callback().Message, sb.String(), menu, telebot.ModeMarkdown)
		return err
	}
	return c.Send(sb.String(), menu, telebot.ModeMarkdown)
}

// handleCronJobsRefresh refreshes the CronJob list.
func (m *Module) handleCronJobsRefresh(c telebot.Context) error {
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "⚠️ Invalid data"})
	}

	namespace, clusterName := parts[0], parts[1]
	return m.sendCronJobList(c, clusterName, namespace)
}
