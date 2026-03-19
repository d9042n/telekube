package testutil

import (
	"sync"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/bot/middleware"
	"gopkg.in/telebot.v3"
)

// SentMessage records a single message sent via the fake context.
type SentMessage struct {
	Text    string
	Options []interface{}
}

// FakeTelebotContext is a minimal telebot.Context implementation for unit tests.
// It records calls to Send/Respond and exposes the recorded messages for assertion.
// It does NOT implement the full telebot.Context surface — only the methods
// that the bot handlers call.
type FakeTelebotContext struct {
	telebot.Context // embed for forward-compat; nil, panics are intentional

	mu       sync.Mutex
	messages []SentMessage
	responds []string

	// State bag (mirrors telebot context.Get/Set behaviour)
	store map[string]interface{}

	// Telegram metadata
	sender   *telebot.User
	chat     *telebot.Chat
	callback *telebot.Callback
	text     string // message text (for slash commands)
}

// NewFakeTelebotContext creates a fake context for a regular user message.
// The provided user is injected into the context so middleware.GetUser works.
func NewFakeTelebotContext(user *entity.User, text string) *FakeTelebotContext {
	tgUser := &telebot.User{
		ID:        user.TelegramID,
		Username:  user.Username,
		FirstName: user.DisplayName,
	}
	fc := &FakeTelebotContext{
		store:  make(map[string]interface{}),
		sender: tgUser,
		chat:   &telebot.Chat{ID: user.TelegramID},
		text:   text,
	}
	// Inject user entity so middleware.GetUser(c) works
	fc.store[string("user")] = user
	return fc
}

// NewFakeCallbackContext creates a fake context that simulates a callback button press.
func NewFakeCallbackContext(user *entity.User, unique, data string) *FakeTelebotContext {
	fc := NewFakeTelebotContext(user, "")
	fc.callback = &telebot.Callback{
		Sender: fc.sender,
		Data:   data,
		Unique: unique,
		Message: &telebot.Message{
			ID:   1,
			Chat: fc.chat,
		},
	}
	return fc
}

// ─── telebot.Context implementation — only the methods used by handlers ───────

func (f *FakeTelebotContext) Send(what interface{}, opts ...interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	text := ""
	switch v := what.(type) {
	case string:
		text = v
	}
	f.messages = append(f.messages, SentMessage{Text: text, Options: opts})
	return nil
}

func (f *FakeTelebotContext) Respond(responses ...*telebot.CallbackResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range responses {
		if r != nil {
			f.responds = append(f.responds, r.Text)
		}
	}
	return nil
}

func (f *FakeTelebotContext) Sender() *telebot.User { return f.sender }
func (f *FakeTelebotContext) Chat() *telebot.Chat   { return f.chat }

func (f *FakeTelebotContext) Callback() *telebot.Callback { return f.callback }

func (f *FakeTelebotContext) Text() string { return f.text }

func (f *FakeTelebotContext) Message() *telebot.Message {
	return &telebot.Message{
		Text:   f.text,
		Chat:   f.chat,
		Sender: f.sender,
	}
}

func (f *FakeTelebotContext) Get(key string) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.store[key]
}

func (f *FakeTelebotContext) Set(key string, val interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[key] = val
}

// ─── Assertion helpers ────────────────────────────────────────────────────────

// SentMessages returns all messages recorded by Send calls.
func (f *FakeTelebotContext) SentMessages() []SentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]SentMessage, len(f.messages))
	copy(out, f.messages)
	return out
}

// LastMessage returns the text of the most recently sent message, or "" if none.
func (f *FakeTelebotContext) LastMessage() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.messages) == 0 {
		return ""
	}
	return f.messages[len(f.messages)-1].Text
}

// LastRespond returns the text of the most recently sent CallbackResponse.
func (f *FakeTelebotContext) LastRespond() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.responds) == 0 {
		return ""
	}
	return f.responds[len(f.responds)-1]
}

// RespondCount returns how many Respond calls have been made.
func (f *FakeTelebotContext) RespondCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.responds)
}

// MessageCount returns how many Send calls have been made.
func (f *FakeTelebotContext) MessageCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// NewTestUser builds a minimal entity.User for use in tests.
func NewTestUser(id int64, username, role string) *entity.User {
	return &entity.User{
		TelegramID:  id,
		Username:    username,
		DisplayName: username,
		Role:        role,
		IsActive:    true,
	}
}

// InjectUser injects an entity.User into a fake telebot context in the same
// way the auth middleware does, so middleware.GetUser(c) returns the given user.
func InjectUser(c *FakeTelebotContext, user *entity.User) {
	// The middleware stores the user under the "user" context key (unexported const
	// in the middleware package). We need to use a workaround: call Set directly
	// with the same string the middleware uses. Keep in sync with middleware.userContextKey.
	_ = middleware.GetUser // ensure import is used
	c.Set("user", user)
}
