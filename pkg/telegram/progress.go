package telegram

import "gopkg.in/telebot.v3"

// SendLoading sends an initial loading indicator message.
func SendLoading(c telebot.Context, text string) (*telebot.Message, error) {
	if text == "" {
		text = "⏳ Loading..."
	}
	return c.Bot().Send(c.Chat(), text)
}

// UpdateMessage updates an existing message with new text.
func UpdateMessage(c telebot.Context, msg *telebot.Message, text string) error {
	_, err := c.Bot().Edit(msg, text)
	return err
}

// UpdateMessageWithMarkup updates an existing message with new text and keyboard.
func UpdateMessageWithMarkup(c telebot.Context, msg *telebot.Message, text string, markup *telebot.ReplyMarkup) error {
	_, err := c.Bot().Edit(msg, text, markup)
	return err
}
