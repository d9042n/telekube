package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderBar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		used     int64
		total    int64
		width    int
		expected string
	}{
		{
			name:     "empty bar when total is zero",
			used:     0,
			total:    0,
			width:    10,
			expected: "[░░░░░░░░░░]",
		},
		{
			name:     "full bar",
			used:     100,
			total:    100,
			width:    10,
			expected: "[██████████]",
		},
		{
			name:     "half bar",
			used:     50,
			total:    100,
			width:    10,
			expected: "[█████░░░░░]",
		},
		{
			name:     "zero usage",
			used:     0,
			total:    100,
			width:    10,
			expected: "[░░░░░░░░░░]",
		},
		{
			name:     "over 100 percent capped",
			used:     150,
			total:    100,
			width:    10,
			expected: "[██████████]",
		},
		{
			name:     "small bar width",
			used:     5,
			total:    10,
			width:    4,
			expected: "[██░░]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := renderBar(tt.used, tt.total, tt.width)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestThresholdEmoji(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ratio    float64
		isCPU    bool
		expected string
	}{
		{
			name:     "CPU below threshold",
			ratio:    0.50,
			isCPU:    true,
			expected: "",
		},
		{
			name:     "CPU at high threshold",
			ratio:    0.90,
			isCPU:    true,
			expected: " 🔴 HIGH",
		},
		{
			name:     "CPU above high threshold",
			ratio:    0.95,
			isCPU:    true,
			expected: " 🔴 HIGH",
		},
		{
			name:     "RAM below all thresholds",
			ratio:    0.50,
			isCPU:    false,
			expected: "",
		},
		{
			name:     "RAM at warning threshold",
			ratio:    0.85,
			isCPU:    false,
			expected: " 🟡",
		},
		{
			name:     "RAM at critical threshold",
			ratio:    0.95,
			isCPU:    false,
			expected: " 🔴 CRITICAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := thresholdEmoji(tt.ratio, tt.isCPU)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{name: "zero bytes", bytes: 0, expected: "0B"},
		{name: "bytes only", bytes: 500, expected: "500B"},
		{name: "kilobytes", bytes: 1024, expected: "1Ki"},
		{name: "megabytes", bytes: 1024 * 1024, expected: "1Mi"},
		{name: "gigabytes", bytes: 1024 * 1024 * 1024, expected: "1.0Gi"},
		{name: "large gigabytes", bytes: 4 * 1024 * 1024 * 1024, expected: "4.0Gi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := formatBytes(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatCPU(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		milliValue int64
		expected   string
	}{
		{name: "sub-core", milliValue: 500, expected: "500m"},
		{name: "one core", milliValue: 1000, expected: "1.0 cores"},
		{name: "multi-core", milliValue: 4000, expected: "4.0 cores"},
		{name: "fraction core", milliValue: 1500, expected: "1.5 cores"},
		{name: "zero", milliValue: 0, expected: "0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := formatCPU(tt.milliValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatResourceName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "requests.cpu", input: "requests.cpu", expected: "CPU Requests"},
		{name: "limits.memory", input: "limits.memory", expected: "RAM Limits"},
		{name: "pods", input: "pods", expected: "Pods"},
		{name: "unknown", input: "something.unknown", expected: "something.unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := formatResourceName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
