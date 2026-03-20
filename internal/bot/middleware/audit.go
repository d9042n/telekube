package middleware

import (
	"time"
	"unicode/utf8"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/oklog/ulid/v2"
	"gopkg.in/telebot.v3"
)

// maxActionRunes limits audit action length to avoid oversized DB entries.
const maxActionRunes = 100

// truncateUTF8 safely truncates a string to at most maxRunes runes,
// ensuring the result is always valid UTF-8.
func truncateUTF8(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}

// Audit logs every bot interaction to the audit trail.
func Audit(auditLogger audit.Logger) telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			user := GetUser(c)
			if user == nil {
				return next(c)
			}

			// Execute handler
			err := next(c)

			// Determine status
			status := entity.AuditStatusSuccess
			errMsg := ""
			if err != nil {
				status = entity.AuditStatusError
				errMsg = err.Error()
			}

			// Determine action: prefer callback unique key over message text.
			// For callbacks, c.Message() returns the original message (response
			// content with emojis/unicode), which is NOT the user action.
			action := "unknown"
			if c.Callback() != nil {
				action = "callback:" + c.Callback().Unique
				if c.Callback().Data != "" {
					action += ":" + truncateUTF8(c.Callback().Data, 60)
				}
			} else if c.Message() != nil && c.Message().Text != "" {
				action = truncateUTF8(c.Message().Text, maxActionRunes)
			}

			// Determine chat type
			chatType := "private"
			if c.Chat() != nil {
				switch c.Chat().Type {
				case telebot.ChatGroup, telebot.ChatSuperGroup:
					chatType = "group"
				case telebot.ChatChannel:
					chatType = "channel"
				}
			}

			chatID := int64(0)
			if c.Chat() != nil {
				chatID = c.Chat().ID
			}

			entry := entity.AuditEntry{
				ID:         ulid.Make().String(),
				UserID:     user.TelegramID,
				Username:   user.Username,
				Action:     action,
				ChatID:     chatID,
				ChatType:   chatType,
				Status:     status,
				Error:      errMsg,
				OccurredAt: time.Now().UTC(),
			}

			auditLogger.Log(entry)

			return err
		}
	}
}
