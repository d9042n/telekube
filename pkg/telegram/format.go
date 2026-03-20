// Package telegram provides Telegram message formatting and UI utilities.
package telegram

import (
	"fmt"
	"strings"
)

// CodeBlock wraps text in a Telegram code block.
func CodeBlock(text string) string {
	return fmt.Sprintf("```\n%s\n```", text)
}

// InlineCode wraps text in inline code.
func InlineCode(text string) string {
	return fmt.Sprintf("`%s`", text)
}

// Bold wraps text in bold markup.
func Bold(text string) string {
	return fmt.Sprintf("*%s*", text)
}

// Italic wraps text in italic markup.
func Italic(text string) string {
	return fmt.Sprintf("_%s_", text)
}

// StatusEmoji returns an emoji representing a K8s pod phase.
func StatusEmoji(phase string) string {
	switch strings.ToLower(phase) {
	case "running":
		return "🟢"
	case "succeeded", "completed":
		return "⚪"
	case "pending", "init", "containercreating", "terminating":
		return "🟡"
	case "failed", "error", "crashloopbackoff", "oomkilled", "imagepullbackoff", "errimagepull":
		return "🔴"
	default:
		return "⚪"
	}
}

// EventEmoji returns an emoji for a K8s event type.
func EventEmoji(eventType string) string {
	switch eventType {
	case "Normal":
		return "✅"
	case "Warning":
		return "⚠️"
	default:
		return "ℹ️"
	}
}

// Truncate truncates text to max length, adding ellipsis if needed.
func Truncate(text string, max int) string {
	if len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

// SanitizeLogs redacts sensitive patterns from log text.
func SanitizeLogs(text string, patterns []string) string {
	for _, pattern := range patterns {
		text = strings.ReplaceAll(text, pattern, "[REDACTED]")
	}
	return text
}

// EscapeMarkdownV2 escapes special characters for Telegram MarkdownV2.
func EscapeMarkdownV2(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}

// SplitMessage splits a long message into chunks that fit Telegram's 4096 char limit.
// It correctly handles code blocks — if a split occurs inside a ``` block,
// the block is closed in the current part and reopened in the next.
func SplitMessage(text string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = 4096
	}
	if len(text) <= maxLen {
		return []string{text}
	}

	var parts []string
	lines := strings.Split(text, "\n")
	var current strings.Builder
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isCodeFence := strings.HasPrefix(trimmed, "```")

		needed := len(line) + 1
		// Reserve space for closing ``` if we're in a code block and need to split
		closingOverhead := 0
		if inCodeBlock {
			closingOverhead = 4 // \n```
		}

		if current.Len()+needed+closingOverhead > maxLen && current.Len() > 0 {
			// Close code block before splitting
			if inCodeBlock {
				current.WriteString("\n```")
			}
			parts = append(parts, current.String())
			current.Reset()
			// Reopen code block in new part
			if inCodeBlock {
				current.WriteString("```\n")
			}
		}

		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(line)

		if isCodeFence {
			inCodeBlock = !inCodeBlock
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// Header returns a formatted section header.
func Header(emoji, title string) string {
	return fmt.Sprintf("%s %s\n━━━━━━━━━━━━━━━━━━", emoji, title)
}
