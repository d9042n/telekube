package notify

import (
	"context"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// --- ShouldNotify tests ---

func TestShouldNotify_NilPref(t *testing.T) {
	assert.True(t, ShouldNotify(nil, "warning", "prod"))
}

func TestShouldNotify_AllowAll(t *testing.T) {
	pref := &entity.NotificationPreference{
		UserID:      1,
		MinSeverity: "info",
	}
	assert.True(t, ShouldNotify(pref, "info", "prod"))
	assert.True(t, ShouldNotify(pref, "warning", "prod"))
	assert.True(t, ShouldNotify(pref, "critical", "prod"))
}

func TestShouldNotify_SeverityFilter_WarningPlus(t *testing.T) {
	pref := &entity.NotificationPreference{
		UserID:      1,
		MinSeverity: "warning",
	}
	assert.False(t, ShouldNotify(pref, "info", "prod"))
	assert.True(t, ShouldNotify(pref, "warning", "prod"))
	assert.True(t, ShouldNotify(pref, "critical", "prod"))
}

func TestShouldNotify_SeverityFilter_CriticalOnly(t *testing.T) {
	pref := &entity.NotificationPreference{
		UserID:      1,
		MinSeverity: "critical",
	}
	assert.False(t, ShouldNotify(pref, "info", "prod"))
	assert.False(t, ShouldNotify(pref, "warning", "prod"))
	assert.True(t, ShouldNotify(pref, "critical", "prod"))
}

func TestShouldNotify_MutedCluster(t *testing.T) {
	pref := &entity.NotificationPreference{
		UserID:        1,
		MinSeverity:   "info",
		MutedClusters: []string{"staging", "dev"},
	}
	assert.False(t, ShouldNotify(pref, "critical", "staging"))
	assert.False(t, ShouldNotify(pref, "warning", "dev"))
	assert.True(t, ShouldNotify(pref, "warning", "prod"))
}

func TestShouldNotify_QuietHours_InWindow(t *testing.T) {
	start := "22:00"
	end := "08:00"
	pref := &entity.NotificationPreference{
		UserID:          1,
		MinSeverity:     "info",
		Timezone:        "UTC",
		QuietHoursStart: &start,
		QuietHoursEnd:   &end,
	}

	// During quiet hours, non-critical should be suppressed
	// We can't easily control time.Now() so we test the logic function directly
	assert.True(t, isInQuietHours("23:00", "22:00", "08:00"))
	assert.True(t, isInQuietHours("01:00", "22:00", "08:00"))
	assert.False(t, isInQuietHours("12:00", "22:00", "08:00"))
	assert.False(t, isInQuietHours("08:00", "22:00", "08:00"))

	// Critical alerts bypass quiet hours
	assert.True(t, ShouldNotify(pref, "critical", "prod"))
	_ = pref // prevent unused warning
}

func TestShouldNotify_QuietHours_SameDayWindow(t *testing.T) {
	// 08:00 — 22:00 (daytime quiet)
	assert.True(t, isInQuietHours("12:00", "08:00", "22:00"))
	assert.False(t, isInQuietHours("23:00", "08:00", "22:00"))
	assert.True(t, isInQuietHours("08:00", "08:00", "22:00"))
	assert.False(t, isInQuietHours("22:00", "08:00", "22:00"))
}

// --- severityLevel tests ---

func TestSeverityLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"critical", 3},
		{"CRITICAL", 3},
		{"warning", 2},
		{"Warning", 2},
		{"info", 1},
		{"INFO", 1},
		{"unknown", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, severityLevel(tt.input))
		})
	}
}

// --- defaultPref tests ---

func TestDefaultPref(t *testing.T) {
	pref := defaultPref(42)
	assert.Equal(t, int64(42), pref.UserID)
	assert.Equal(t, "info", pref.MinSeverity)
	assert.Equal(t, "UTC", pref.Timezone)
	assert.Nil(t, pref.QuietHoursStart)
	assert.Nil(t, pref.QuietHoursEnd)
	assert.Empty(t, pref.MutedClusters)
}

// --- Module interface tests ---

func TestModule_Interface(t *testing.T) {
	m := &Module{healthy: true}
	assert.Equal(t, "notify", m.Name())
	assert.Equal(t, "User notification preferences", m.Description())
	assert.Len(t, m.Commands(), 1)
	assert.Equal(t, "/notify", m.Commands()[0].Command)
}

func TestModule_Health(t *testing.T) {
	m := &Module{healthy: true}
	assert.Equal(t, entity.HealthStatusHealthy, m.Health())

	m.healthy = false
	assert.Equal(t, entity.HealthStatusUnhealthy, m.Health())
}

func TestModule_StartStop(t *testing.T) {
	m := &Module{logger: zap.NewNop()}
	assert.NoError(t, m.Start(context.TODO()))
	assert.NoError(t, m.Stop(context.TODO()))
}

// --- isInQuietHours edge cases ---

func TestIsInQuietHours_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		now      string
		start    string
		end      string
		expected bool
	}{
		{"before midnight wrap", "22:30", "22:00", "06:00", true},
		{"after midnight wrap", "03:00", "22:00", "06:00", true},
		{"outside midnight wrap", "12:00", "22:00", "06:00", false},
		{"at start", "22:00", "22:00", "06:00", true},
		{"at end (not in)", "06:00", "22:00", "06:00", false},
		{"same day at start", "09:00", "09:00", "17:00", true},
		{"same day middle", "13:00", "09:00", "17:00", true},
		{"same day outside", "20:00", "09:00", "17:00", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isInQuietHours(tt.now, tt.start, tt.end))
		})
	}
}

// --- Combined filter tests ---

func TestShouldNotify_Combined(t *testing.T) {
	start := "23:00"
	end := "07:00"
	pref := &entity.NotificationPreference{
		UserID:          1,
		MinSeverity:     "warning",
		MutedClusters:   []string{"dev"},
		Timezone:        "UTC",
		QuietHoursStart: &start,
		QuietHoursEnd:   &end,
	}

	// Info on prod — blocked by severity
	assert.False(t, ShouldNotify(pref, "info", "prod"))

	// Warning on dev — blocked by muted cluster
	assert.False(t, ShouldNotify(pref, "warning", "dev"))

	// Critical on dev — blocked by muted cluster (cluster check comes first)
	assert.False(t, ShouldNotify(pref, "critical", "dev"))

	// Warning on prod — allowed (severity passes, not muted)
	assert.True(t, ShouldNotify(pref, "warning", "prod"))
}

// Verify quiet hours are timezone-aware but functional with invalid timezone
func TestShouldNotify_InvalidTimezone(t *testing.T) {
	start := "22:00"
	end := "08:00"
	pref := &entity.NotificationPreference{
		UserID:          1,
		MinSeverity:     "info",
		Timezone:        "Invalid/Timezone",
		QuietHoursStart: &start,
		QuietHoursEnd:   &end,
	}

	// With invalid timezone, time.LoadLocation fails, so quiet hours are skipped
	// and alerts should pass through
	assert.True(t, ShouldNotify(pref, "info", "prod"))
}

func TestShouldNotify_EmptyMinSeverity(t *testing.T) {
	pref := &entity.NotificationPreference{
		UserID:      1,
		MinSeverity: "",
	}
	// Empty min severity means no filtering
	assert.True(t, ShouldNotify(pref, "info", "prod"))
	assert.True(t, ShouldNotify(pref, "critical", "prod"))
}

// Keep compile time check
var _ = time.Now
