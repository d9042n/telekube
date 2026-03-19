package telegram

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusEmoji(t *testing.T) {
	t.Parallel()

	tests := []struct {
		phase    string
		expected string
	}{
		{"Running", "🟢"},
		{"running", "🟢"},
		{"Succeeded", "⚪"},
		{"Completed", "⚪"},
		{"Pending", "🟡"},
		{"ContainerCreating", "🟡"},
		{"Terminating", "🟡"},
		{"Failed", "🔴"},
		{"CrashLoopBackOff", "🔴"},
		{"OOMKilled", "🔴"},
		{"Error", "🔴"},
		{"Unknown", "⚪"},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, StatusEmoji(tt.phase))
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		text     string
		max      int
		expected string
	}{
		{"short text", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 8, "hello..."},
		{"very short max", "hello", 2, "he"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, Truncate(tt.text, tt.max))
		})
	}
}

func TestCodeBlock(t *testing.T) {
	t.Parallel()
	result := CodeBlock("hello")
	assert.Equal(t, "```\nhello\n```", result)
}

func TestBold(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "*hello*", Bold("hello"))
}

func TestInlineCode(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "`hello`", InlineCode("hello"))
}

func TestSplitMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		text       string
		maxLen     int
		wantParts  int
	}{
		{"short text", "hello", 100, 1},
		{"exact limit", "hello", 5, 1},
		{"needs split", "line1\nline2\nline3", 11, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parts := SplitMessage(tt.text, tt.maxLen)
			assert.Len(t, parts, tt.wantParts)
		})
	}
}

func TestSanitizeLogs(t *testing.T) {
	t.Parallel()
	text := "connecting to password=secret123 host=db"
	result := SanitizeLogs(text, []string{"secret123"})
	assert.Contains(t, result, "[REDACTED]")
	assert.NotContains(t, result, "secret123")
}

func TestEventEmoji(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "✅", EventEmoji("Normal"))
	assert.Equal(t, "⚠️", EventEmoji("Warning"))
	assert.Equal(t, "ℹ️", EventEmoji("Other"))
}

func TestHeader(t *testing.T) {
	t.Parallel()
	result := Header("📦", "Pods")
	assert.Contains(t, result, "📦 Pods")
	assert.Contains(t, result, "━━━")
}
