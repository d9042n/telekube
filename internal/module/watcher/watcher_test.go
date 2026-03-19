package watcher

import (
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestAlertSeverityEmoji(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		severity AlertSeverity
		expected string
	}{
		{name: "critical", severity: SeverityCritical, expected: "🔴"},
		{name: "warning", severity: SeverityWarning, expected: "🟡"},
		{name: "unknown", severity: AlertSeverity("info"), expected: "⚪"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.severity.Emoji())
		})
	}
}

func TestPodWatcherMuteAlert(t *testing.T) {
	t.Parallel()

	w := &PodWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   defaultCooldown,
	}

	alertKey := "cluster/ns/pod/OOMKilled"

	// Muting should set the cache entry so that cooldown extends
	w.muteAlert(alertKey, 1*time.Hour)

	w.mu.RLock()
	cachedTime, exists := w.alertCache[alertKey]
	w.mu.RUnlock()

	assert.True(t, exists)
	// The muted time should be in the future relative to now minus cooldown
	assert.True(t, time.Since(cachedTime) < 0 || time.Since(cachedTime) < 1*time.Hour)
}

func TestNodeWatcherMuteAlert(t *testing.T) {
	t.Parallel()

	w := &NodeWatcher{
		alertCache: make(map[string]time.Time),
		cooldown:   defaultCooldown,
	}

	alertKey := "cluster/node/NotReady"

	w.muteAlert(alertKey, 1*time.Hour)

	w.mu.RLock()
	_, exists := w.alertCache[alertKey]
	w.mu.RUnlock()

	assert.True(t, exists)
}

func TestCronJobWatcherDefaults(t *testing.T) {
	t.Parallel()

	w := NewCronJobWatcher(nil, nil, nil, config.TelegramConfig{}, CronJobWatcherConfig{}, nil)
	assert.Equal(t, defaultCronCheckInterval, w.watcherCfg.CheckInterval)
	assert.Equal(t, defaultCooldown, w.cooldown)
}

func TestCronJobWatcher_isExcluded(t *testing.T) {
	t.Parallel()

	w := &CronJobWatcher{
		watcherCfg: CronJobWatcherConfig{
			ExcludeNamespaces: []string{"kube-system", "monitoring"},
		},
	}

	assert.True(t, w.isExcluded("kube-system"))
	assert.True(t, w.isExcluded("monitoring"))
	assert.False(t, w.isExcluded("production"))
	assert.False(t, w.isExcluded(""))
}

func TestCertWatcherDefaults(t *testing.T) {
	t.Parallel()

	w := NewCertWatcher(nil, nil, nil, config.TelegramConfig{}, CertWatcherConfig{}, nil)
	assert.Equal(t, defaultCertCheckInterval, w.watcherCfg.CheckInterval)
	assert.Equal(t, defaultAlertDaysBefore, w.watcherCfg.AlertDaysBefore)
	assert.Equal(t, defaultCriticalDaysBefore, w.watcherCfg.CriticalDaysBefore)
}

func TestCertWatcher_isExcluded(t *testing.T) {
	t.Parallel()

	w := &CertWatcher{
		watcherCfg: CertWatcherConfig{
			ExcludeNamespaces: []string{"kube-system"},
		},
	}

	assert.True(t, w.isCertExcluded("kube-system"))
	assert.False(t, w.isCertExcluded("production"))
}

func TestPVCWatcherDefaults(t *testing.T) {
	t.Parallel()

	w := NewPVCWatcher(nil, nil, nil, config.TelegramConfig{}, PVCWatcherConfig{}, nil)
	assert.Equal(t, defaultPVCCheckInterval, w.watcherCfg.CheckInterval)
	assert.Equal(t, float64(defaultPVCWarningPct), w.watcherCfg.WarningThreshold)
	assert.Equal(t, float64(defaultPVCCriticalPct), w.watcherCfg.CriticalThreshold)
}

func TestPVCWatcher_isExcluded(t *testing.T) {
	t.Parallel()

	w := &PVCWatcher{
		watcherCfg: PVCWatcherConfig{
			ExcludeNamespaces: []string{"kube-system", "kube-public"},
		},
	}

	assert.True(t, w.isPVCExcluded("kube-system"))
	assert.True(t, w.isPVCExcluded("kube-public"))
	assert.False(t, w.isPVCExcluded("production"))
}
