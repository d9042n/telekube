//go:build e2e

// Package e2e contains end-to-end test helpers and test cases.
// These tests require Docker to run and are excluded from plain `go test ./...`.
// Run with: go test ./test/e2e/... -tags=e2e -timeout=20m
package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SentMessage records a message sent by the bot to the fake Telegram server.
type SentMessage struct {
	ChatID    int64
	Text      string
	ParseMode string
	MessageID int
	EditedAt  time.Time
}

// CallbackResponse records an answerCallbackQuery response from the bot.
type CallbackResponse struct {
	CallbackQueryID string
	Text            string
	ShowAlert       bool
}

// fakeTelegramUpdate is the shape of a Telegram Update injected through the queue.
type fakeTelegramUpdate struct {
	UpdateID int            `json:"update_id"`
	Message  *fakeMessage   `json:"message,omitempty"`
	Callback *fakeCallback  `json:"callback_query,omitempty"`
}

type fakeMessage struct {
	MessageID int        `json:"message_id"`
	From      *fakeUser  `json:"from"`
	Chat      *fakeChat  `json:"chat"`
	Date      int64      `json:"date"`
	Text      string     `json:"text"`
}

type fakeCallback struct {
	ID      string       `json:"id"`
	From    *fakeUser    `json:"from"`
	Message *fakeMessage `json:"message"`
	Data    string       `json:"data"`
}

type fakeUser struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	FirstName    string `json:"first_name"`
	IsBot        bool   `json:"is_bot"`
	LanguageCode string `json:"language_code"`
}

type fakeChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// FakeTelegramServer is an in-process HTTP server that implements the subset of
// the Telegram Bot API that telebot.v3 needs to operate.
//
// It lets tests:
//   - Inject fake updates (commands, callbacks) via SendUpdate / SendMessage / SendCallback
//   - Record outgoing bot messages via SentMessages / WaitForMessage
//   - Reset state between tests via ClearMessages
type FakeTelegramServer struct {
	server *httptest.Server
	token  string

	mu                sync.Mutex
	updateQueue       []fakeTelegramUpdate
	sentMessages      []SentMessage
	callbackResponses []CallbackResponse
	nextUpdateID      int64
	nextMsgID         int64
}

// NewFakeTelegramServer creates and starts a fake Telegram HTTP server.
// The server is automatically closed when the test ends (no cleanup needed here;
// callers must call Close).
func NewFakeTelegramServer() *FakeTelegramServer {
	f := &FakeTelegramServer{
		token: "TEST_BOT_TOKEN",
	}

	mux := http.NewServeMux()

	// Match all /bot<token>/method routes.
	mux.HandleFunc("/", f.dispatch)

	f.server = httptest.NewServer(mux)
	return f
}

// URL returns the base URL of the fake server (no trailing slash).
func (f *FakeTelegramServer) URL() string {
	return f.server.URL
}

// Token returns the fake bot token.
func (f *FakeTelegramServer) Token() string {
	return f.token
}

// Close shuts down the underlying HTTP server.
func (f *FakeTelegramServer) Close() {
	f.server.Close()
}

// dispatch routes requests to the right handler method.
// telebot calls <base_url>/bot<token>/<method>
func (f *FakeTelegramServer) dispatch(w http.ResponseWriter, r *http.Request) {
	// Strip the /bot<token>/ prefix.
	path := r.URL.Path
	prefix := "/bot" + f.token + "/"
	if !strings.HasPrefix(path, prefix) {
		http.NotFound(w, r)
		return
	}
	method := strings.TrimPrefix(path, prefix)

	switch method {
	case "getMe":
		f.handleGetMe(w, r)
	case "getUpdates":
		f.handleGetUpdates(w, r)
	case "sendMessage":
		f.handleSendMessage(w, r)
	case "editMessageText":
		f.handleEditMessageText(w, r)
	case "answerCallbackQuery":
		f.handleAnswerCallbackQuery(w, r)
	case "deleteMessage":
		f.handleDeleteMessage(w, r)
	case "sendChatAction":
		f.handleSendChatAction(w, r)
	default:
		// Return ok:true for unknown methods to avoid bot crashes.
		writeOK(w, true)
	}
}

// handleGetMe returns a minimal bot info object.
func (f *FakeTelegramServer) handleGetMe(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, map[string]interface{}{
		"id":         int64(12345),
		"is_bot":     true,
		"username":   "telekube_test_bot",
		"first_name": "Telekube Test",
	})
}

// handleGetUpdates drains the update queue.
func (f *FakeTelegramServer) handleGetUpdates(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	updates := f.updateQueue
	f.updateQueue = nil
	f.mu.Unlock()

	writeOK(w, updates)
}

// handleSendMessage records an outbound message.
func (f *FakeTelegramServer) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var payload struct {
		ChatID    interface{} `json:"chat_id"`
		Text      string      `json:"text"`
		ParseMode string      `json:"parse_mode"`
	}
	_ = json.Unmarshal(body, &payload)

	chatID := toInt64(payload.ChatID)
	msgID := int(atomic.AddInt64(&f.nextMsgID, 1))

	f.mu.Lock()
	f.sentMessages = append(f.sentMessages, SentMessage{
		ChatID:    chatID,
		Text:      payload.Text,
		ParseMode: payload.ParseMode,
		MessageID: msgID,
	})
	f.mu.Unlock()

	writeOK(w, map[string]interface{}{
		"message_id": msgID,
		"chat":       map[string]interface{}{"id": chatID},
		"text":       payload.Text,
		"date":       time.Now().Unix(),
	})
}

// handleEditMessageText records a message edit.
func (f *FakeTelegramServer) handleEditMessageText(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var payload struct {
		ChatID    interface{} `json:"chat_id"`
		MessageID int         `json:"message_id"`
		Text      string      `json:"text"`
		ParseMode string      `json:"parse_mode"`
	}
	_ = json.Unmarshal(body, &payload)

	chatID := toInt64(payload.ChatID)

	f.mu.Lock()
	f.sentMessages = append(f.sentMessages, SentMessage{
		ChatID:    chatID,
		Text:      payload.Text,
		ParseMode: payload.ParseMode,
		MessageID: payload.MessageID,
		EditedAt:  time.Now(),
	})
	f.mu.Unlock()

	writeOK(w, map[string]interface{}{
		"message_id": payload.MessageID,
		"chat":       map[string]interface{}{"id": chatID},
		"text":       payload.Text,
		"date":       time.Now().Unix(),
	})
}

// handleAnswerCallbackQuery records the callback query answer text.
func (f *FakeTelegramServer) handleAnswerCallbackQuery(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var payload struct {
		CallbackQueryID string `json:"callback_query_id"`
		Text            string `json:"text"`
		ShowAlert       bool   `json:"show_alert"`
	}
	_ = json.Unmarshal(body, &payload)

	if payload.Text != "" {
		f.mu.Lock()
		f.callbackResponses = append(f.callbackResponses, CallbackResponse{
			CallbackQueryID: payload.CallbackQueryID,
			Text:            payload.Text,
			ShowAlert:       payload.ShowAlert,
		})
		f.mu.Unlock()
	}
	writeOK(w, true)
}

// handleDeleteMessage is a no-op — always returns ok.
func (f *FakeTelegramServer) handleDeleteMessage(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, true)
}

// handleSendChatAction is a no-op — always returns ok.
func (f *FakeTelegramServer) handleSendChatAction(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, true)
}

// ─── Inject helpers ──────────────────────────────────────────────────────────

// InjectTextMessage injects a fake text command from userID into the update queue.
func (f *FakeTelegramServer) InjectTextMessage(userID int64, username, text string) {
	updateID := int(atomic.AddInt64(&f.nextUpdateID, 1))
	msgID := int(atomic.AddInt64(&f.nextMsgID, 1))

	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateQueue = append(f.updateQueue, fakeTelegramUpdate{
		UpdateID: updateID,
		Message: &fakeMessage{
			MessageID: msgID,
			From:      &fakeUser{ID: userID, Username: username, FirstName: username},
			Chat:      &fakeChat{ID: userID, Type: "private"},
			Date:      time.Now().Unix(),
			Text:      text,
		},
	})
}

// InjectCallback injects a fake inline keyboard callback from userID.
func (f *FakeTelegramServer) InjectCallback(userID int64, username, unique, data string) {
	updateID := int(atomic.AddInt64(&f.nextUpdateID, 1))
	msgID := int(atomic.AddInt64(&f.nextMsgID, 1))

	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateQueue = append(f.updateQueue, fakeTelegramUpdate{
		UpdateID: updateID,
		Callback: &fakeCallback{
			ID:   fmt.Sprintf("cb_%d", updateID),
			From: &fakeUser{ID: userID, Username: username, FirstName: username},
			Message: &fakeMessage{
				MessageID: msgID,
				Chat:      &fakeChat{ID: userID, Type: "private"},
				Date:      time.Now().Unix(),
			},
			Data: fmt.Sprintf("\f%s|%s", unique, data),
		},
	})
}

// ─── Observation helpers ──────────────────────────────────────────────────────

// SentMessages returns a snapshot of all messages sent by the bot.
func (f *FakeTelegramServer) SentMessages() []SentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]SentMessage, len(f.sentMessages))
	copy(out, f.sentMessages)
	return out
}

// MessagesTo returns all messages sent to a specific chat ID.
func (f *FakeTelegramServer) MessagesTo(chatID int64) []SentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []SentMessage
	for _, m := range f.sentMessages {
		if m.ChatID == chatID {
			out = append(out, m)
		}
	}
	return out
}

// LastMessageTo returns the text of the last message sent to chatID, or "".
func (f *FakeTelegramServer) LastMessageTo(chatID int64) string {
	msgs := f.MessagesTo(chatID)
	if len(msgs) == 0 {
		return ""
	}
	return msgs[len(msgs)-1].Text
}

// WaitForMessage polls until a message matching predicate is found (or timeout).
// Returns the matching message text.
func (f *FakeTelegramServer) WaitForMessage(timeout time.Duration, predicate func(string) bool) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msgs := f.SentMessages()
		for _, m := range msgs {
			if predicate(m.Text) {
				return m.Text, true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", false
}

// WaitForMessageTo polls until a message is sent to chatID matching predicate.
func (f *FakeTelegramServer) WaitForMessageTo(chatID int64, timeout time.Duration, predicate func(string) bool) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msgs := f.MessagesTo(chatID)
		for _, m := range msgs {
			if predicate(m.Text) {
				return m.Text, true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", false
}

// ClearMessages resets all recorded messages — call between tests for isolation.
func (f *FakeTelegramServer) ClearMessages() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentMessages = nil
	f.callbackResponses = nil
}

// CallbackResponses returns a snapshot of all callback query responses.
func (f *FakeTelegramServer) CallbackResponses() []CallbackResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]CallbackResponse, len(f.callbackResponses))
	copy(out, f.callbackResponses)
	return out
}

// WaitForCallbackResponse polls for a callback toast matching predicate.
func (f *FakeTelegramServer) WaitForCallbackResponse(timeout time.Duration, predicate func(string) bool) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for _, cr := range f.callbackResponses {
			if predicate(cr.Text) {
				f.mu.Unlock()
				return cr.Text, true
			}
		}
		f.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
	return "", false
}

// ─── Private helpers ──────────────────────────────────────────────────────────

// writeOK writes a Telegram API success envelope.
func writeOK(w http.ResponseWriter, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"ok":     true,
		"result": result,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// toInt64 converts a JSON-unmarshalled value (which may be float64 or string) to int64.
// telebot sends chat_id as a string via Recipient.Recipient().
func toInt64(v interface{}) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	default:
		return 0
	}
}
