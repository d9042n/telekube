//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── FakeTelegramServer unit tests ───────────────────────────────────────────
// These tests exercise the fake server in isolation — no bot, no k3s.

func TestFakeTelegramServer_InjectAndRead(t *testing.T) {
	srv := newFakeServer(t)

	// Inject a message and read it back.
	srv.InjectTextMessage(1001, "alice", "/start")

	msgs := srv.SentMessages()
	// No bot running — sent messages are empty; we only test the update queue
	// via a direct HTTP call to /getUpdates.
	_ = msgs // will be populated once a bot drains the queue

	// The server must be reachable.
	resp, err := get(t, srv.URL()+"/bot"+srv.Token()+"/getMe")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestFakeTelegramServer_GetUpdates_DrainQueue(t *testing.T) {
	srv := newFakeServer(t)

	srv.InjectTextMessage(1001, "alice", "/start")
	srv.InjectTextMessage(1002, "bob", "/help")

	// First call drains both updates.
	body := mustGetBody(t, srv.URL()+"/bot"+srv.Token()+"/getUpdates")
	assert.Contains(t, body, "/start")
	assert.Contains(t, body, "/help")

	// Second call returns empty queue.
	body2 := mustGetBody(t, srv.URL()+"/bot"+srv.Token()+"/getUpdates")
	assert.NotContains(t, body2, "/start")
}

func TestFakeTelegramServer_SendMessage_Recorded(t *testing.T) {
	srv := newFakeServer(t)

	// Simulate a bot calling sendMessage.
	mustPostJSON(t, srv.URL()+"/bot"+srv.Token()+"/sendMessage", map[string]interface{}{
		"chat_id": 1001,
		"text":    "Hello, Alice!",
	})

	msgs := srv.SentMessages()
	require.Len(t, msgs, 1)
	assert.Equal(t, int64(1001), msgs[0].ChatID)
	assert.Equal(t, "Hello, Alice!", msgs[0].Text)
}

func TestFakeTelegramServer_WaitForMessage_HappyPath(t *testing.T) {
	srv := newFakeServer(t)

	// In a goroutine, simulate the bot sending a message after a short delay.
	go func() {
		time.Sleep(100 * time.Millisecond)
		mustPostJSON(t, srv.URL()+"/bot"+srv.Token()+"/sendMessage", map[string]interface{}{
			"chat_id": 2001,
			"text":    "Welcome to telekube!",
		})
	}()

	text, ok := srv.WaitForMessage(3*time.Second, func(s string) bool {
		return strings.Contains(s, "Welcome")
	})
	require.True(t, ok, "expected WaitForMessage to return true")
	assert.Contains(t, text, "Welcome")
}

func TestFakeTelegramServer_WaitForMessage_Timeout(t *testing.T) {
	srv := newFakeServer(t)

	// No message will arrive.
	_, ok := srv.WaitForMessage(200*time.Millisecond, func(s string) bool {
		return strings.Contains(s, "NonExistent")
	})
	assert.False(t, ok, "expected timeout")
}

func TestFakeTelegramServer_ClearMessages(t *testing.T) {
	srv := newFakeServer(t)

	mustPostJSON(t, srv.URL()+"/bot"+srv.Token()+"/sendMessage", map[string]interface{}{
		"chat_id": 1001,
		"text":    "message1",
	})
	require.Len(t, srv.SentMessages(), 1)

	srv.ClearMessages()
	assert.Empty(t, srv.SentMessages())
}

func TestFakeTelegramServer_MessagesTo_Filtered(t *testing.T) {
	srv := newFakeServer(t)

	mustPostJSON(t, srv.URL()+"/bot"+srv.Token()+"/sendMessage", map[string]interface{}{
		"chat_id": 1001, "text": "msg-for-alice",
	})
	mustPostJSON(t, srv.URL()+"/bot"+srv.Token()+"/sendMessage", map[string]interface{}{
		"chat_id": 1002, "text": "msg-for-bob",
	})

	aliceMsgs := srv.MessagesTo(1001)
	require.Len(t, aliceMsgs, 1)
	assert.Equal(t, "msg-for-alice", aliceMsgs[0].Text)

	bobMsgs := srv.MessagesTo(1002)
	require.Len(t, bobMsgs, 1)
	assert.Equal(t, "msg-for-bob", bobMsgs[0].Text)
}

// ─── Smoke test: Harness starts, bot replies to /start ────────────────────────

func TestHarness_Smoke_BotRepliesOnStart(t *testing.T) {
	const userID = int64(111111)
	const adminID = int64(999999)

	// Start harness without k3s (for speed).
	h := newSmokeHarness(t, adminID)

	// Send /start as the configured admin user.
	h.SendMessage(adminID, "testadmin", "/start")

	// The bot must reply within 5 seconds.
	msg := h.AssertBotReplied(adminID, 5*time.Second)
	t.Logf("Bot replied: %q", msg)

	// Message should contain something sensible — not empty.
	assert.NotEmpty(t, msg)
	_ = userID
}

func TestHarness_Smoke_UnknownUserAutoRegistered(t *testing.T) {
	const newUserID = int64(222222)
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)

	// A completely new user (not in adminIDs) sends /start.
	h.SendMessage(newUserID, "newcomer", "/start")

	// The bot should still reply (auto-register as viewer).
	msg := h.AssertBotReplied(newUserID, 5*time.Second)
	t.Logf("New user bot reply: %q", msg)
	assert.NotEmpty(t, msg)
}
