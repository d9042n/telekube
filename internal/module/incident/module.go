// Package incident implements the incident timeline builder module.
package incident

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/bot/keyboard"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/storage"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TimelineEvent represents a single event in the incident timeline.
type TimelineEvent struct {
	Timestamp time.Time
	Emoji     string
	Category  string // "k8s", "user"
	Summary   string
}

// Timeline is a chronological list of events.
type Timeline struct {
	Events    []TimelineEvent
	Namespace string
	Cluster   string
	From      time.Time
	To        time.Time
}

// Sort orders events chronologically.
func (t *Timeline) Sort() {
	sort.Slice(t.Events, func(i, j int) bool {
		return t.Events[i].Timestamp.Before(t.Events[j].Timestamp)
	})
}

// Append adds an event.
func (t *Timeline) Append(e TimelineEvent) {
	t.Events = append(t.Events, e)
}

// collectOpts bundles collection options.
type collectOpts struct {
	Cluster   string
	Namespace string
	From      time.Time
	To        time.Time
}

// Collector gathers events from multiple data sources.
type Collector struct {
	clusters cluster.Manager
	storage  storage.Storage
	logger   *zap.Logger
}

func newCollector(clusters cluster.Manager, store storage.Storage, logger *zap.Logger) *Collector {
	return &Collector{clusters: clusters, storage: store, logger: logger}
}

func (c *Collector) collect(ctx context.Context, opts collectOpts) (*Timeline, error) {
	tl := &Timeline{
		Namespace: opts.Namespace,
		Cluster:   opts.Cluster,
		From:      opts.From,
		To:        opts.To,
	}

	c.collectK8sEvents(ctx, tl, opts)
	c.collectAuditEntries(ctx, tl, opts)

	tl.Sort()
	return tl, nil
}

func (c *Collector) collectK8sEvents(ctx context.Context, tl *Timeline, opts collectOpts) {
	client, err := c.clusters.ClientSet(opts.Cluster)
	if err != nil {
		c.logger.Warn("failed to get clientset for incident collector", zap.Error(err))
		return
	}

	events, err := client.CoreV1().Events(opts.Namespace).List(ctx, metav1.ListOptions{Limit: 200})
	if err != nil {
		c.logger.Warn("listing k8s events for incident", zap.Error(err))
		return
	}

	for _, e := range events.Items {
		ts := e.LastTimestamp.Time
		if ts.IsZero() {
			ts = e.EventTime.Time
		}
		if ts.IsZero() || ts.Before(opts.From) || ts.After(opts.To) {
			continue
		}
		tl.Append(TimelineEvent{
			Timestamp: ts,
			Emoji:     k8sEventEmoji(e),
			Category:  "k8s",
			Summary:   fmt.Sprintf("%s — %s (%s)", e.InvolvedObject.Name, e.Reason, e.Message),
		})
	}
}

func (c *Collector) collectAuditEntries(ctx context.Context, tl *Timeline, opts collectOpts) {
	filter := storage.AuditFilter{
		Cluster:  &opts.Cluster,
		From:     &opts.From,
		To:       &opts.To,
		Page:     1,
		PageSize: 100,
	}
	if opts.Namespace != "" {
		filter.Namespace = &opts.Namespace
	}

	entries, _, err := c.storage.Audit().List(ctx, filter)
	if err != nil {
		c.logger.Warn("listing audit entries for incident", zap.Error(err))
		return
	}

	for _, e := range entries {
		tl.Append(TimelineEvent{
			Timestamp: e.OccurredAt,
			Emoji:     "👤",
			Category:  "user",
			Summary:   fmt.Sprintf("@%s — %s %s", e.Username, e.Action, e.Resource),
		})
	}
}

func k8sEventEmoji(e corev1.Event) string {
	if e.Type == "Warning" {
		return "⚠️"
	}
	switch e.Reason {
	case "OOMKilling":
		return "💥"
	case "Started":
		return "▶️"
	case "Pulled", "Scheduled":
		return "📦"
	case "BackOff":
		return "🔁"
	default:
		return "📋"
	}
}

// Module implements the Telegram incident timeline module.
type Module struct {
	clusters cluster.Manager
	storage  storage.Storage
	logger   *zap.Logger
	cbs      *keyboard.CallbackStore
}

// NewModule creates a new incident module.
func NewModule(clusters cluster.Manager, store storage.Storage, logger *zap.Logger) *Module {
	return &Module{clusters: clusters, storage: store, logger: logger, cbs: keyboard.NewCallbackStore()}
}

func (m *Module) Name() string        { return "incident" }
func (m *Module) Description() string { return "Incident timeline builder" }

func (m *Module) Register(bot *telebot.Bot, _ *telebot.Group) {
	bot.Handle("/incident", m.handleIncident)
	bot.Handle(&telebot.Btn{Unique: "inc_ns_select"}, m.resolve(m.handleNsSelect))
	bot.Handle(&telebot.Btn{Unique: "inc_window_select"}, m.resolve(m.handleWindowSelect))
}

// resolve is middleware that resolves shortened callback data.
func (m *Module) resolve(handler telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {
		if cb := c.Callback(); cb != nil && cb.Data != "" {
			cb.Data = m.cbs.Resolve(cb.Data)
		}
		return handler(c)
	}
}

// sd stores callback data, returning short key if too long.
func (m *Module) sd(data string) string {
	return m.cbs.Store(data)
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("incident module started")
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }

func (m *Module) Health() entity.HealthStatus { return entity.HealthStatusHealthy }

func (m *Module) Commands() []module.CommandInfo {
	return []module.CommandInfo{
		{
			Command:     "/incident",
			Description: "Build incident timeline from K8s events and audit log",
			Permission:  "kubernetes.pods.events",
			ChatType:    "all",
		},
	}
}

func (m *Module) handleIncident(c telebot.Context) error {
	clusters := m.clusters.List()
	if len(clusters) == 0 {
		return c.Send("⚠️ No clusters configured")
	}
	cl := clusters[0]

	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	// Dynamically list namespaces from the cluster
	namespaces := m.listClusterNamespaces(cl.Name)
	var nsRow []telebot.Btn
	for _, ns := range namespaces {
		nsRow = append(nsRow, menu.Data(ns, "inc_ns_select", m.sd(cl.Name+"|"+ns)))
		if len(nsRow) == 2 {
			rows = append(rows, menu.Row(nsRow...))
			nsRow = nil
		}
	}
	if len(nsRow) > 0 {
		rows = append(rows, menu.Row(nsRow...))
	}
	rows = append(rows, menu.Row(menu.Data("All Namespaces", "inc_ns_select", m.sd(cl.Name+"|"))))
	menu.Inline(rows...)

	return c.Send("🚨 Incident Timeline Builder\n\nSelect namespace:", menu)
}

// listClusterNamespaces dynamically queries namespaces from a cluster.
// Returns a limited set for the UI (max 8 namespaces excluding system ones).
func (m *Module) listClusterNamespaces(clusterName string) []string {
	cs, err := m.clusters.ClientSet(clusterName)
	if err != nil {
		return []string{"production", "staging"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return []string{"production", "staging"}
	}

	exclude := map[string]bool{
		"kube-system":    true,
		"kube-public":    true,
		"kube-node-lease": true,
	}

	names := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		if exclude[ns.Name] {
			continue
		}
		names = append(names, ns.Name)
		if len(names) >= 8 {
			break
		}
	}
	if len(names) == 0 {
		return []string{"default"}
	}
	return names
}

func (m *Module) handleNsSelect(c telebot.Context) error {
	_ = c.Respond(&telebot.CallbackResponse{})
	data := c.Callback().Data
	parts := strings.SplitN(data, "|", 2)
	if len(parts) != 2 {
		return nil
	}
	clusterName, ns := parts[0], parts[1]

	menu := &telebot.ReplyMarkup{}
	base := fmt.Sprintf("%s|%s", clusterName, ns)
	menu.Inline(
		menu.Row(
			menu.Data("⏱️ Last 15min", "inc_window_select", m.sd(base+"|15m")),
			menu.Data("⏱️ Last 30min", "inc_window_select", m.sd(base+"|30m")),
		),
		menu.Row(
			menu.Data("⏱️ Last 1h", "inc_window_select", m.sd(base+"|1h")),
			menu.Data("⏱️ Last 4h", "inc_window_select", m.sd(base+"|4h")),
		),
	)
	return c.Edit("Select time window:", menu)
}

func (m *Module) handleWindowSelect(c telebot.Context) error {
	_ = c.Respond(&telebot.CallbackResponse{})
	data := c.Callback().Data
	parts := strings.SplitN(data, "|", 3)
	if len(parts) != 3 {
		return nil
	}
	clusterName, ns, windowStr := parts[0], parts[1], parts[2]

	duration, err := time.ParseDuration(windowStr)
	if err != nil {
		return c.Send("❌ Invalid time window")
	}

	_ = c.Edit("🔍 Building incident timeline...")

	now := time.Now().UTC()
	opts := collectOpts{
		Cluster:   clusterName,
		Namespace: ns,
		From:      now.Add(-duration),
		To:        now,
	}

	collector := newCollector(m.clusters, m.storage, m.logger)
	tl, err := collector.collect(context.Background(), opts)
	if err != nil {
		return c.Edit(fmt.Sprintf("❌ Error: %v", err))
	}

	text := formatTimeline(tl)
	menu := &telebot.ReplyMarkup{}
	menu.Inline(menu.Row(
		menu.Data("🔄 Refresh", "inc_window_select", m.sd(data)),
		menu.Data("◀️ Back", "inc_ns_select", m.sd(clusterName+"|"+ns)),
	))

	return c.Edit(text, menu)
}

func formatTimeline(tl *Timeline) string {
	var sb strings.Builder
	ns := tl.Namespace
	if ns == "" {
		ns = "All Namespaces"
	}
	sb.WriteString(fmt.Sprintf("🚨 Incident Timeline — %s (last %s)\n", ns,
		tl.To.Sub(tl.From).Round(time.Minute)))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("Cluster: %s | %s — %s UTC\n\n",
		tl.Cluster,
		tl.From.UTC().Format("2006-01-02 15:04"),
		tl.To.UTC().Format("15:04"),
	))

	if len(tl.Events) == 0 {
		sb.WriteString("No events found in this window.\n")
	}

	events := tl.Events
	if len(events) > 30 {
		sb.WriteString(fmt.Sprintf("⚠️ Showing first 30 of %d events\n\n", len(events)))
		events = events[:30]
	}

	for _, e := range events {
		ts := e.Timestamp.UTC().Format("15:04")
		sb.WriteString(fmt.Sprintf("%s  %s %s\n", ts, e.Emoji, e.Summary))
	}

	sb.WriteString("\n═══════════════════════════════════════════\n")
	sb.WriteString(fmt.Sprintf("Total events: %d\n", len(tl.Events)))

	return sb.String()
}
