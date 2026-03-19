// Package notify provides the /notify Telegram command for managing
// per-user notification preferences (severity filter, muting, quiet hours).
package notify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/storage"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// Module implements the notification preferences module.
type Module struct {
	store   storage.NotificationPrefRepository
	logger  *zap.Logger
	healthy bool
}

// NewModule creates a new notification preferences module.
func NewModule(
	store storage.NotificationPrefRepository,
	logger *zap.Logger,
) *Module {
	return &Module{
		store:   store,
		logger:  logger,
		healthy: true,
	}
}

func (m *Module) Name() string        { return "notify" }
func (m *Module) Description() string { return "User notification preferences" }

func (m *Module) Register(bot *telebot.Bot, _ *telebot.Group) {
	bot.Handle("/notify", m.handleNotify)

	// Callbacks
	bot.Handle(&telebot.Btn{Unique: "notify_severity"}, m.handleSetSeverity)
	bot.Handle(&telebot.Btn{Unique: "notify_severity_set"}, m.handleSeverityConfirm)
	bot.Handle(&telebot.Btn{Unique: "notify_quiet"}, m.handleQuietHours)
	bot.Handle(&telebot.Btn{Unique: "notify_quiet_set"}, m.handleQuietHoursConfirm)
	bot.Handle(&telebot.Btn{Unique: "notify_quiet_clear"}, m.handleQuietHoursClear)
	bot.Handle(&telebot.Btn{Unique: "notify_mute_cluster"}, m.handleMuteCluster)
	bot.Handle(&telebot.Btn{Unique: "notify_mute_cluster_toggle"}, m.handleMuteClusterToggle)
	bot.Handle(&telebot.Btn{Unique: "notify_back"}, m.handleBack)
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("notification preferences module started")
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	m.logger.Info("notification preferences module stopped")
	return nil
}

func (m *Module) Health() entity.HealthStatus {
	if m.healthy {
		return entity.HealthStatusHealthy
	}
	return entity.HealthStatusUnhealthy
}

func (m *Module) Commands() []module.CommandInfo {
	return []module.CommandInfo{
		{
			Command:     "/notify",
			Description: "Manage your notification preferences",
			Permission:  "", // All users can manage their own prefs
			ChatType:    "all",
		},
	}
}

// ---------- Handlers ----------

// handleNotify shows the notification settings menu.
func (m *Module) handleNotify(c telebot.Context) error {
	ctx := context.Background()
	userID := c.Sender().ID

	pref, err := m.store.Get(ctx, userID)
	if err != nil {
		// Use defaults
		pref = defaultPref(userID)
	}

	return m.showMainMenu(c, pref)
}

func (m *Module) showMainMenu(c telebot.Context, pref *entity.NotificationPreference) error {
	var sb strings.Builder
	sb.WriteString("🔔 *Notification Settings*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Current settings
	severity := pref.MinSeverity
	if severity == "" {
		severity = "info"
	}
	sb.WriteString(fmt.Sprintf("📊 Min severity: `%s`\n", severity))

	if len(pref.MutedClusters) > 0 {
		sb.WriteString(fmt.Sprintf("🔇 Muted clusters: %s\n", strings.Join(pref.MutedClusters, ", ")))
	} else {
		sb.WriteString("🔇 Muted clusters: none\n")
	}

	if len(pref.MutedAlerts) > 0 {
		sb.WriteString(fmt.Sprintf("🔇 Muted alerts: %s\n", strings.Join(pref.MutedAlerts, ", ")))
	}

	if pref.QuietHoursStart != nil && pref.QuietHoursEnd != nil {
		tz := pref.Timezone
		if tz == "" {
			tz = "UTC"
		}
		sb.WriteString(fmt.Sprintf("🌙 Quiet hours: %s — %s (%s)\n", *pref.QuietHoursStart, *pref.QuietHoursEnd, tz))
	} else {
		sb.WriteString("🌙 Quiet hours: disabled\n")
	}

	menu := &telebot.ReplyMarkup{}
	btnSeverity := menu.Data("📊 Severity Filter", "notify_severity")
	btnQuiet := menu.Data("🌙 Quiet Hours", "notify_quiet")
	btnMuteCluster := menu.Data("🔇 Mute Cluster", "notify_mute_cluster")
	menu.Inline(
		menu.Row(btnSeverity),
		menu.Row(btnQuiet),
		menu.Row(btnMuteCluster),
	)

	// If edit is possible, edit; otherwise send
	if c.Callback() != nil {
		return c.Edit(sb.String(), menu, telebot.ModeMarkdown)
	}
	return c.Send(sb.String(), menu, telebot.ModeMarkdown)
}

// handleSetSeverity shows severity options.
func (m *Module) handleSetSeverity(c telebot.Context) error {
	menu := &telebot.ReplyMarkup{}
	btnInfo := menu.Data("ℹ️ Info (all alerts)", "notify_severity_set", "info")
	btnWarning := menu.Data("⚠️ Warning+", "notify_severity_set", "warning")
	btnCritical := menu.Data("🔴 Critical only", "notify_severity_set", "critical")
	btnBack := menu.Data("⬅ Back", "notify_back")
	menu.Inline(
		menu.Row(btnInfo),
		menu.Row(btnWarning),
		menu.Row(btnCritical),
		menu.Row(btnBack),
	)

	return c.Edit("📊 *Minimum Severity*\n\nChoose the minimum severity level for alerts.\nAlerts below this level will be silenced for you.", menu, telebot.ModeMarkdown)
}

// handleSeverityConfirm saves the severity preference.
func (m *Module) handleSeverityConfirm(c telebot.Context) error {
	ctx := context.Background()
	userID := c.Sender().ID
	severity := c.Callback().Data

	pref, err := m.store.Get(ctx, userID)
	if err != nil {
		pref = defaultPref(userID)
	}

	pref.MinSeverity = severity
	if err := m.store.Upsert(ctx, pref); err != nil {
		m.logger.Error("failed to save notification pref", zap.Error(err))
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Failed to save"})
	}

	_ = c.Respond(&telebot.CallbackResponse{Text: fmt.Sprintf("✅ Severity set to: %s", severity)})
	return m.showMainMenu(c, pref)
}

// handleQuietHours shows quiet hours options.
func (m *Module) handleQuietHours(c telebot.Context) error {
	menu := &telebot.ReplyMarkup{}
	btn1 := menu.Data("🌙 22:00 — 08:00", "notify_quiet_set", "22:00|08:00")
	btn2 := menu.Data("🌙 23:00 — 07:00", "notify_quiet_set", "23:00|07:00")
	btn3 := menu.Data("🌙 00:00 — 06:00", "notify_quiet_set", "00:00|06:00")
	btnClear := menu.Data("🔔 Disable quiet hours", "notify_quiet_clear")
	btnBack := menu.Data("⬅ Back", "notify_back")
	menu.Inline(
		menu.Row(btn1),
		menu.Row(btn2),
		menu.Row(btn3),
		menu.Row(btnClear),
		menu.Row(btnBack),
	)

	return c.Edit("🌙 *Quiet Hours*\n\nDuring quiet hours, only *critical* alerts will be delivered.\n\nSelect a time window:", menu, telebot.ModeMarkdown)
}

// handleQuietHoursConfirm saves quiet hours preference.
func (m *Module) handleQuietHoursConfirm(c telebot.Context) error {
	ctx := context.Background()
	userID := c.Sender().ID
	data := c.Callback().Data

	parts := strings.SplitN(data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Invalid selection"})
	}

	pref, err := m.store.Get(ctx, userID)
	if err != nil {
		pref = defaultPref(userID)
	}

	pref.QuietHoursStart = &parts[0]
	pref.QuietHoursEnd = &parts[1]

	if err := m.store.Upsert(ctx, pref); err != nil {
		m.logger.Error("failed to save notification pref", zap.Error(err))
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Failed to save"})
	}

	_ = c.Respond(&telebot.CallbackResponse{Text: fmt.Sprintf("✅ Quiet hours: %s — %s", parts[0], parts[1])})
	return m.showMainMenu(c, pref)
}

// handleQuietHoursClear removes quiet hours.
func (m *Module) handleQuietHoursClear(c telebot.Context) error {
	ctx := context.Background()
	userID := c.Sender().ID

	pref, err := m.store.Get(ctx, userID)
	if err != nil {
		pref = defaultPref(userID)
	}

	pref.QuietHoursStart = nil
	pref.QuietHoursEnd = nil

	if err := m.store.Upsert(ctx, pref); err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Failed to save"})
	}

	_ = c.Respond(&telebot.CallbackResponse{Text: "✅ Quiet hours disabled"})
	return m.showMainMenu(c, pref)
}

// handleMuteCluster shows cluster mute/unmute options.
func (m *Module) handleMuteCluster(c telebot.Context) error {
	ctx := context.Background()
	userID := c.Sender().ID

	pref, err := m.store.Get(ctx, userID)
	if err != nil {
		pref = defaultPref(userID)
	}

	var sb strings.Builder
	sb.WriteString("🔇 *Mute/Unmute Clusters*\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	mutedSet := make(map[string]bool)
	for _, c := range pref.MutedClusters {
		mutedSet[c] = true
	}

	sb.WriteString("Select a cluster to toggle mute:\n")

	// We don't have access to cluster manager here, so offer common clusters
	// In practice, user enters cluster names via config
	menu := &telebot.ReplyMarkup{}
	var rows []telebot.Row

	if len(pref.MutedClusters) > 0 {
		sb.WriteString("\nCurrently muted:\n")
		for _, c := range pref.MutedClusters {
			sb.WriteString(fmt.Sprintf("  🔇 %s\n", c))
			btn := menu.Data(fmt.Sprintf("🔔 Unmute %s", c), "notify_mute_cluster_toggle", c)
			rows = append(rows, menu.Row(btn))
		}
	} else {
		sb.WriteString("\nNo clusters muted.\n")
	}

	sb.WriteString("\n_Note: Alerts from muted clusters are silenced for you._")

	btnBack := menu.Data("⬅ Back", "notify_back")
	rows = append(rows, menu.Row(btnBack))
	menu.Inline(rows...)

	return c.Edit(sb.String(), menu, telebot.ModeMarkdown)
}

// handleMuteClusterToggle toggles mute for a specific cluster.
func (m *Module) handleMuteClusterToggle(c telebot.Context) error {
	ctx := context.Background()
	userID := c.Sender().ID
	clusterName := c.Callback().Data

	pref, err := m.store.Get(ctx, userID)
	if err != nil {
		pref = defaultPref(userID)
	}

	// Toggle
	found := false
	var updated []string
	for _, c := range pref.MutedClusters {
		if c == clusterName {
			found = true
			continue // Remove (unmute)
		}
		updated = append(updated, c)
	}
	if !found {
		updated = append(updated, clusterName) // Add (mute)
	}
	pref.MutedClusters = updated

	if err := m.store.Upsert(ctx, pref); err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "❌ Failed to save"})
	}

	action := "muted"
	if found {
		action = "unmuted"
	}
	_ = c.Respond(&telebot.CallbackResponse{Text: fmt.Sprintf("✅ Cluster %s %s", clusterName, action)})
	return m.showMainMenu(c, pref)
}

// handleBack returns to the main notification menu.
func (m *Module) handleBack(c telebot.Context) error {
	ctx := context.Background()
	pref, err := m.store.Get(ctx, c.Sender().ID)
	if err != nil {
		pref = defaultPref(c.Sender().ID)
	}
	return m.showMainMenu(c, pref)
}

// ShouldNotify checks whether a user should receive an alert based on their preferences.
// This is called by watchers before sending alerts.
func ShouldNotify(pref *entity.NotificationPreference, severity, clusterName string) bool {
	if pref == nil {
		return true
	}

	// Check muted clusters
	for _, m := range pref.MutedClusters {
		if m == clusterName {
			return false
		}
	}

	// Check severity filter
	if pref.MinSeverity != "" {
		if severityLevel(severity) < severityLevel(pref.MinSeverity) {
			return false
		}
	}

	// Check quiet hours
	if pref.QuietHoursStart != nil && pref.QuietHoursEnd != nil && severity != "critical" {
		tz := pref.Timezone
		if tz == "" {
			tz = "UTC"
		}
		loc, err := time.LoadLocation(tz)
		if err == nil {
			now := time.Now().In(loc)
			hourMin := now.Format("15:04")
			start := *pref.QuietHoursStart
			end := *pref.QuietHoursEnd

			if isInQuietHours(hourMin, start, end) {
				return false
			}
		}
	}

	return true
}

func severityLevel(s string) int {
	switch strings.ToLower(s) {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func isInQuietHours(now, start, end string) bool {
	if start <= end {
		// e.g., 08:00 — 22:00
		return now >= start && now < end
	}
	// Wraps midnight, e.g., 22:00 — 08:00
	return now >= start || now < end
}

func defaultPref(userID int64) *entity.NotificationPreference {
	return &entity.NotificationPreference{
		UserID:      userID,
		MinSeverity: "info",
		Timezone:    "UTC",
	}
}
