package briefing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseCronTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		schedule string
		expHour  int
		expMin   int
	}{
		{name: "standard 8am", schedule: "0 8 * * *", expHour: 8, expMin: 0},
		{name: "9:30 AM", schedule: "30 9 * * *", expHour: 9, expMin: 30},
		{name: "midnight", schedule: "0 0 * * *", expHour: 0, expMin: 0},
		{name: "invalid schedule", schedule: "invalid", expHour: 8, expMin: 0},
		{name: "empty schedule", schedule: "", expHour: 8, expMin: 0},
		{name: "6 PM", schedule: "0 18 * * *", expHour: 18, expMin: 0},
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

func TestBriefingReportFormat(t *testing.T) {
	t.Parallel()

	report := &BriefingReport{
		GeneratedAt: time.Date(2025, 3, 17, 8, 0, 0, 0, time.UTC),
		Clusters: []ClusterReport{
			{
				Name:        "production",
				NodesReady:  3,
				NodesTotal:  3,
				PodsRunning: 42,
				PodsFailed:  1,
				PodsPending: 2,
				AvgCPU:      35.5,
				AvgRAM:      62.3,
				AlertsCount: 5,
			},
			{
				Name:        "staging",
				NodesReady:  1,
				NodesTotal:  1,
				PodsRunning: 10,
				PodsFailed:  0,
				PodsPending: 0,
				AvgCPU:      12.0,
				AvgRAM:      25.0,
				AlertsCount: 0,
			},
		},
		Activity: ActivityReport{
			TotalActions: 20,
			Restarts:     3,
			Scales:       1,
			Deploys:      0,
		},
	}

	result := report.Format()

	// Verify key sections are present
	assert.Contains(t, result, "Daily Briefing")
	assert.Contains(t, result, "production")
	assert.Contains(t, result, "staging")
	assert.Contains(t, result, "3/3 Ready")
	assert.Contains(t, result, "42 Running")
	assert.Contains(t, result, "1 Failed")
	assert.Contains(t, result, "2 Pending")
	assert.Contains(t, result, "36%")
	assert.Contains(t, result, "62%")
	assert.Contains(t, result, "5 in last 24h")
	assert.Contains(t, result, "0 in last 24h")
	assert.Contains(t, result, "20 actions")
	assert.Contains(t, result, "Restarts: 3")
	assert.Contains(t, result, "Scales:   1")
}

func TestBriefingReportFormatNodesStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		nodesReady      int
		nodesTotal      int
		expectedContain string
	}{
		{
			name:            "all ready",
			nodesReady:      3,
			nodesTotal:      3,
			expectedContain: "✅",
		},
		{
			name:            "some not ready",
			nodesReady:      2,
			nodesTotal:      3,
			expectedContain: "🟡",
		},
		{
			name:            "none ready",
			nodesReady:      0,
			nodesTotal:      3,
			expectedContain: "🔴",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			report := &BriefingReport{
				GeneratedAt: time.Now(),
				Clusters: []ClusterReport{
					{
						Name:       "test",
						NodesReady: tt.nodesReady,
						NodesTotal: tt.nodesTotal,
					},
				},
			}
			result := report.Format()
			assert.Contains(t, result, tt.expectedContain)
		})
	}
}
