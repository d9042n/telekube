package bot

import (
	"gopkg.in/telebot.v3"
)

// Notifier sends alert messages to Telegram chats via the bot.
type Notifier struct {
	bot *telebot.Bot
}

// NewNotifier creates a new Notifier from a Bot instance.
func NewNotifier(b *Bot) *Notifier {
	return &Notifier{bot: b.tele}
}

// SendAlert sends a message to a specific chat ID.
func (n *Notifier) SendAlert(chatID int64, text string, markup *telebot.ReplyMarkup) error {
	recipient := &telebot.Chat{ID: chatID}
	opts := []interface{}{telebot.ModeMarkdown}
	if markup != nil {
		opts = append(opts, markup)
	}
	_, err := n.bot.Send(recipient, text, opts...)
	return err
}
