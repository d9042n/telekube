package middleware

import (
	"context"
	"time"

	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

type contextKey string

const userContextKey contextKey = "user"

// GetUser extracts the user entity from the telebot context.
func GetUser(c telebot.Context) *entity.User {
	v := c.Get(string(userContextKey))
	if v == nil {
		return nil
	}
	u, ok := v.(*entity.User)
	if !ok {
		return nil
	}
	return u
}

// Auth authenticates users and auto-registers on first contact.
func Auth(store storage.Storage, cfg config.TelegramConfig, logger *zap.Logger) telebot.MiddlewareFunc {
	allowedChats := make(map[int64]bool)
	for _, id := range cfg.AllowedChats {
		allowedChats[id] = true
	}

	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			sender := c.Sender()
			if sender == nil {
				return nil // Ignore messages without sender
			}

			// Check allowed chats
			if len(allowedChats) > 0 {
				chatID := c.Chat().ID
				if !allowedChats[chatID] && chatID != sender.ID {
					// Chat not allowed and not a private chat with the sender
					return nil
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			// Try to get existing user
			user, err := store.Users().GetByTelegramID(ctx, sender.ID)
			if err != nil {
				if err != storage.ErrNotFound {
					logger.Error("failed to get user",
						zap.Int64("telegram_id", sender.ID),
						zap.Error(err),
					)
					return c.Send("⚠️ Internal error. Please try again.")
				}

				// Auto-register: determine role
				role := entity.RoleViewer
				for _, adminID := range cfg.AdminIDs {
					if sender.ID == adminID {
						role = entity.RoleAdmin
						break
					}
				}

				user = &entity.User{
					TelegramID:  sender.ID,
					Username:    sender.Username,
					DisplayName: displayName(sender),
					Role:        role,
					IsActive:    true,
					CreatedAt:   time.Now().UTC(),
					UpdatedAt:   time.Now().UTC(),
				}

				if err := store.Users().Upsert(ctx, user); err != nil {
					logger.Error("failed to register user",
						zap.Int64("telegram_id", sender.ID),
						zap.Error(err),
					)
					return c.Send("⚠️ Internal error. Please try again.")
				}

				logger.Info("new user registered",
					zap.Int64("telegram_id", sender.ID),
					zap.String("username", sender.Username),
					zap.String("role", role),
				)
			}

			// Update username/display_name if changed
			if user.Username != sender.Username || user.DisplayName != displayName(sender) {
				user.Username = sender.Username
				user.DisplayName = displayName(sender)
				_ = store.Users().Upsert(ctx, user) // Best effort
			}

			// Inject user into context
			c.Set(string(userContextKey), user)

			return next(c)
		}
	}
}

func displayName(sender *telebot.User) string {
	name := sender.FirstName
	if sender.LastName != "" {
		name += " " + sender.LastName
	}
	return name
}
