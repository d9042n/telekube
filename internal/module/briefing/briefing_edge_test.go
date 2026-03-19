package briefing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ─── parseCronTime edge cases ────────────────────────────────────────────────

func TestParseCronTime_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		schedule string
		expHour  int
		expMin   int
	}{
		{name: "standard 8am", schedule: "0 8 * * *", expHour: 8, expMin: 0},
		{name: "midnight", schedule: "0 0 * * *", expHour: 0, expMin: 0},
		{name: "11:59 PM", schedule: "59 23 * * *", expHour: 23, expMin: 59},
		{name: "minute only", schedule: "30", expHour: 8, expMin: 0},
		{name: "invalid", schedule: "abc def", expHour: 8, expMin: 0},
		{name: "empty string", schedule: "", expHour: 8, expMin: 0},
		{name: "extra fields", schedule: "15 9 1 * * *", expHour: 9, expMin: 15},
		{name: "negative minute", schedule: "-1 8 * * *", expHour: 8, expMin: -1},
		{name: "large hour", schedule: "0 25 * * *", expHour: 25, expMin: 0},
		{name: "spaces only", schedule: "   ", expHour: 8, expMin: 0},
		{name: "tabs", schedule: "\t\t", expHour: 8, expMin: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hour, minute := parseCronTime(tt.schedule)
			assert.Equal(t, tt.expHour, hour)
			assert.Equal(t, tt.expMin, minute)
		})
	}
}

// ─── BriefingReport.Format edge cases ────────────────────────────────────────

func TestBriefingReportFormat_EmptyClusters(t *testing.T) {
	t.Parallel()

	report := &BriefingReport{
		GeneratedAt: time.Now(),
		Clusters:    nil,
		Activity:    ActivityReport{TotalActions: 0},
	}

	result := report.Format()
	assert.Contains(t, result, "Daily Briefing")
	assert.Contains(t, result, "0 actions")
	assert.NotContains(t, result, "Restarts:")
	assert.NotContains(t, result, "Scales:")
	assert.NotContains(t, result, "Deploys:")
}

func TestBriefingReportFormat_NoFailedNoPending(t *testing.T) {
	t.Parallel()

	report := &BriefingReport{
		GeneratedAt: time.Now(),
		Clusters: []ClusterReport{
			{
				Name:        "staging",
				NodesReady:  2,
				NodesTotal:  2,
				PodsRunning: 10,
				PodsFailed:  0,
				PodsPending: 0,
			},
		},
	}

	result := report.Format()
	assert.Contains(t, result, "10 Running")
	assert.NotContains(t, result, "Failed")
	assert.NotContains(t, result, "Pending")
}

func TestBriefingReportFormat_ZeroCPURAM_NotShown(t *testing.T) {
	t.Parallel()

	report := &BriefingReport{
		GeneratedAt: time.Now(),
		Clusters: []ClusterReport{
			{
				Name:   "test",
				AvgCPU: 0,
				AvgRAM: 0,
			},
		},
	}

	result := report.Format()
	assert.NotContains(t, result, "CPU:")
	assert.NotContains(t, result, "RAM:")
}

func TestBriefingReportFormat_AllNodesDown(t *testing.T) {
	t.Parallel()

	report := &BriefingReport{
		GeneratedAt: time.Now(),
		Clusters: []ClusterReport{
			{Name: "prod", NodesReady: 0, NodesTotal: 5},
		},
	}

	result := report.Format()
	assert.Contains(t, result, "🔴")
	assert.Contains(t, result, "0/5 Ready")
}

func TestBriefingReportFormat_AllActivityTypes(t *testing.T) {
	t.Parallel()

	report := &BriefingReport{
		GeneratedAt: time.Now(),
		Activity: ActivityReport{
			TotalActions: 100,
			Restarts:     10,
			Scales:       5,
			Deploys:      3,
		},
	}

	result := report.Format()
	assert.Contains(t, result, "100 actions")
	assert.Contains(t, result, "Restarts: 10")
	assert.Contains(t, result, "Scales:   5")
	assert.Contains(t, result, "Deploys:  3")
}

func TestBriefingReportFormat_ContainsFooter(t *testing.T) {
	t.Parallel()

	report := &BriefingReport{
		GeneratedAt: time.Now(),
	}

	result := report.Format()
	assert.Contains(t, result, "Chúc team ngày mới")
}

// ─── Scheduler edge cases ────────────────────────────────────────────────────

func TestScheduler_Stop_Idempotent(t *testing.T) {
	t.Parallel()

	s := &Scheduler{done: make(chan struct{})}
	s.Stop()
	// Second call should not panic.
	s.Stop()
}

func TestScheduler_NewScheduler_Fields(t *testing.T) {
	t.Parallel()

	cfg := Config{Schedule: "0 9 * * *", Timezone: "Asia/Ho_Chi_Minh"}
	s := NewScheduler(nil, nil, cfg, []int64{111, 222}, nil)
	assert.NotNil(t, s.done)
	assert.Equal(t, cfg, s.cfg)
	assert.Equal(t, []int64{111, 222}, s.chats)
}
