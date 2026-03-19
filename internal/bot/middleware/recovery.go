// Package middleware provides the Telegram bot middleware chain.
package middleware

import (
	"runtime/debug"

	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// Recovery catches panics in handlers to prevent bot crashes.
func Recovery(logger *zap.Logger) telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("handler panic recovered",
						zap.Any("panic", r),
						zap.String("stack", string(debug.Stack())),
					)
					_ = c.Send("⚠️ Internal error occurred. The team has been notified.")
				}
			}()
			return next(c)
		}
	}
}
