package middleware

import (
	"time"

	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/oklog/ulid/v2"
	"gopkg.in/telebot.v3"
)

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

			// Determine action from message text or callback
			action := "unknown"
			if c.Message() != nil && c.Message().Text != "" {
				action = c.Message().Text
				if len(action) > 100 {
					action = action[:100]
				}
			} else if c.Callback() != nil {
				action = "callback:" + c.Callback().Unique
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
