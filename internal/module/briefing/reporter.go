package briefing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/storage"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BriefingReport holds the generated report data.
type BriefingReport struct {
	GeneratedAt time.Time
	Clusters    []ClusterReport
	Activity    ActivityReport
}

// ClusterReport holds per-cluster status.
type ClusterReport struct {
	Name         string
	NodesReady   int
	NodesTotal   int
	PodsRunning  int
	PodsFailed   int
	PodsPending  int
	AvgCPU       float64
	AvgRAM       float64
	AlertsCount  int
}

// ActivityReport holds recent activity summary.
type ActivityReport struct {
	TotalActions int
	Restarts     int
	Scales       int
	Deploys      int
}

// Reporter generates briefing reports from cluster data.
type Reporter struct {
	clusters cluster.Manager
	audit    audit.Logger
	logger   *zap.Logger
}

// NewReporter creates a new briefing reporter.
func NewReporter(
	clusters cluster.Manager,
	auditLogger audit.Logger,
	logger *zap.Logger,
) *Reporter {
	return &Reporter{
		clusters: clusters,
		audit:    auditLogger,
		logger:   logger,
	}
}

// Generate creates a full briefing report across all clusters.
func (r *Reporter) Generate(ctx context.Context) (*BriefingReport, error) {
	report := &BriefingReport{
		GeneratedAt: time.Now(),
	}

	for _, c := range r.clusters.List() {
		clusterReport := r.generateClusterReport(ctx, c.Name)
		report.Clusters = append(report.Clusters, clusterReport)
	}

	report.Activity = r.getLast24hActivity(ctx)
	return report, nil
}

// generateClusterReport generates a report for a single cluster.
func (r *Reporter) generateClusterReport(ctx context.Context, clusterName string) ClusterReport {
	cr := ClusterReport{
		Name: clusterName,
	}

	clientSet, err := r.clusters.ClientSet(clusterName)
	if err != nil {
		r.logger.Error("failed to get clientset for briefing",
			zap.String("cluster", clusterName),
			zap.Error(err),
		)
		return cr
	}

	// Node status
	nodes, err := clientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		cr.NodesTotal = len(nodes.Items)
		for _, node := range nodes.Items {
			for _, cond := range node.Status.Conditions {
				if cond.Type == "Ready" && cond.Status == "True" {
					cr.NodesReady++
				}
			}
		}
	}

	// Pod status
	pods, err := clientSet.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			switch pod.Status.Phase {
			case "Running":
				cr.PodsRunning++
			case "Failed":
				cr.PodsFailed++
			case "Pending":
				cr.PodsPending++
			}
		}
	}

	// CPU/RAM averages from metrics
	metricsClient, err := r.clusters.MetricsClient(clusterName)
	if err == nil && metricsClient != nil {
		nodeMetrics, err := metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
		if err == nil && nodes != nil && len(nodes.Items) > 0 {
			totalCPURatio := 0.0
			totalRAMRatio := 0.0
			count := 0

			for _, nm := range nodeMetrics.Items {
				for _, node := range nodes.Items {
					if node.Name == nm.Name {
						cpuCap := node.Status.Allocatable.Cpu().MilliValue()
						ramCap := node.Status.Allocatable.Memory().Value()

						if cpuCap > 0 {
							totalCPURatio += float64(nm.Usage.Cpu().MilliValue()) / float64(cpuCap)
						}
						if ramCap > 0 {
							totalRAMRatio += float64(nm.Usage.Memory().Value()) / float64(ramCap)
						}
						count++
					}
				}
			}

			if count > 0 {
				cr.AvgCPU = totalCPURatio / float64(count) * 100
				cr.AvgRAM = totalRAMRatio / float64(count) * 100
			}
		}
	}

	// Alert count (last 24h)
	since := time.Now().Add(-24 * time.Hour)
	actionStr := "alert."
	entries, _, _ := r.audit.Query(ctx, storage.AuditFilter{
		Cluster: &clusterName,
		Action:  &actionStr,
		From:    &since,
		Page:    1,
		PageSize: 1000,
	})
	cr.AlertsCount = len(entries)

	return cr
}

// getLast24hActivity queries recent audit entries for activity summary.
func (r *Reporter) getLast24hActivity(ctx context.Context) ActivityReport {
	since := time.Now().Add(-24 * time.Hour)
	entries, _, _ := r.audit.Query(ctx, storage.AuditFilter{
		From:     &since,
		Page:     1,
		PageSize: 1000,
	})

	activity := ActivityReport{
		TotalActions: len(entries),
	}

	for _, entry := range entries {
		switch {
		case strings.HasPrefix(entry.Action, "pod.restart"):
			activity.Restarts++
		case strings.Contains(entry.Action, "scale"):
			activity.Scales++
		case strings.Contains(entry.Action, "deploy"):
			activity.Deploys++
		}
	}

	return activity
}

// Format renders the report as a Telegram message.
func (r *BriefingReport) Format() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("🌤️ *Telekube Daily Briefing* — %s\n", r.GeneratedAt.Format("2006-01-02 15:04")))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	for _, cr := range r.Clusters {
		nodesEmoji := "✅"
		if cr.NodesReady < cr.NodesTotal {
			nodesEmoji = "🟡"
		}
		if cr.NodesReady == 0 && cr.NodesTotal > 0 {
			nodesEmoji = "🔴"
		}

		sb.WriteString(fmt.Sprintf("📍 *Cluster: %s*\n", cr.Name))
		sb.WriteString(fmt.Sprintf("   Nodes:  %d/%d Ready %s\n", cr.NodesReady, cr.NodesTotal, nodesEmoji))

		podLine := fmt.Sprintf("   Pods:   %d Running", cr.PodsRunning)
		if cr.PodsFailed > 0 {
			podLine += fmt.Sprintf(", %d Failed 🟡", cr.PodsFailed)
		}
		if cr.PodsPending > 0 {
			podLine += fmt.Sprintf(", %d Pending", cr.PodsPending)
		}
		sb.WriteString(podLine + "\n")

		if cr.AvgCPU > 0 {
			sb.WriteString(fmt.Sprintf("   CPU:    %.0f%% avg across nodes\n", cr.AvgCPU))
		}
		if cr.AvgRAM > 0 {
			sb.WriteString(fmt.Sprintf("   RAM:    %.0f%% avg across nodes\n", cr.AvgRAM))
		}

		if cr.AlertsCount > 0 {
			sb.WriteString(fmt.Sprintf("   Alerts: %d in last 24h\n", cr.AlertsCount))
		} else {
			sb.WriteString("   Alerts: 0 in last 24h\n")
		}
		sb.WriteString("\n")
	}

	// Activity
	sb.WriteString("🔄 *Last 24h Activity:*\n")
	sb.WriteString(fmt.Sprintf("   Total:    %d actions\n", r.Activity.TotalActions))
	if r.Activity.Restarts > 0 {
		sb.WriteString(fmt.Sprintf("   Restarts: %d\n", r.Activity.Restarts))
	}
	if r.Activity.Scales > 0 {
		sb.WriteString(fmt.Sprintf("   Scales:   %d\n", r.Activity.Scales))
	}
	if r.Activity.Deploys > 0 {
		sb.WriteString(fmt.Sprintf("   Deploys:  %d\n", r.Activity.Deploys))
	}

	sb.WriteString("\nChúc team ngày mới ít Bug! 🚀")

	return sb.String()
}
