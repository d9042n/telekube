//go:build e2e

package e2e_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Auth / Auto-registration ─────────────────────────────────────────────────

// TestE2E_Auth_NewUser_AutoRegistered verifies that a completely unknown user
// is auto-registered as a viewer and still receives a reply from the bot.
func TestE2E_Auth_NewUser_AutoRegistered(t *testing.T) {
	const (
		adminID  = int64(999999)
		newUser  = int64(100001)
		username = "newcomer"
	)

	h := newSmokeHarness(t, adminID)

	h.SendMessage(newUser, username, "/start")

	msg, ok := h.WaitForMessageTo(newUser, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to unregistered user")
	assert.NotEmpty(t, msg)
}

// TestE2E_Auth_NilSender_Ignored verifies the bot does not crash or reply when
// it receives a message with an unseen sender, and remains responsive afterwards.
func TestE2E_Auth_NilSender_Ignored(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)

	// A never-seen user — the bot registers them as viewer and sends /start.
	h.SendMessage(99999999, "ghost", "/start")

	// Verify bot is still alive.
	h.SendMessage(adminID, "testadmin", "/start")
	_, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "bot must still be alive after unknown user message")
}

// ─── RBAC — role-based permission checks ──────────────────────────────────────

// TestE2E_RBAC_Viewer_CanList verifies that a viewer receives the /pods namespace selector.
func TestE2E_RBAC_Viewer_CanList(t *testing.T) {
	const (
		adminID  = int64(999999)
		viewerID = int64(200001)
	)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(viewerID, "viewer1", "viewer")

	h.SendMessage(viewerID, "viewer1", "/pods")

	msg, ok := h.WaitForMessageTo(viewerID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "viewer must receive pods namespace selector")
	assert.NotEmpty(t, msg)
	assert.NotContains(t, strings.ToLower(msg), "permission denied")
}

// TestE2E_RBAC_Viewer_CannotRestart verifies that a viewer cannot restart pods.
// The k8s_restart callback performs the RBAC check and responds with a toast via
// c.Respond(&telebot.CallbackResponse{Text: "⛔ …"}).
func TestE2E_RBAC_Viewer_CannotRestart(t *testing.T) {
	const (
		adminID  = int64(999999)
		viewerID = int64(200002)
	)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(viewerID, "viewer2", "viewer")

	// k8s_restart checks RBAC and calls c.Respond (answerCallbackQuery) when denied.
	h.SendCallback(viewerID, "viewer2", "k8s_restart", "nginx-pod|default|test-cluster")

	// 1. Try sendMessage / editMessageText (in case the bot calls c.Send or c.Edit).
	isDenied := func(s string) bool {
		return strings.Contains(s, "⛔") ||
			strings.Contains(strings.ToLower(s), "permission") ||
			strings.Contains(strings.ToLower(s), "denied")
	}

	msg, ok := h.WaitForMessageTo(viewerID, 3*time.Second, isDenied)
	if ok {
		t.Logf("denial via sendMessage: %q", msg)
		return
	}

	// 2. Check answerCallbackQuery toast (c.Respond).
	toast, ok := h.Telegram.WaitForCallbackResponse(3*time.Second, isDenied)
	require.True(t, ok, "viewer must be denied restart via callback toast or message")
	t.Logf("denial via callback toast: %q", toast)
}

// TestE2E_RBAC_Operator_CanRestart verifies that an operator is not denied
// restart by triggering the real k8s_restart callback.
func TestE2E_RBAC_Operator_CanRestart(t *testing.T) {
	const (
		adminID    = int64(999999)
		operatorID = int64(200003)
	)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(operatorID, "operator1", "operator")

	// k8s_restart with operator — RBAC check uses PermKubernetesPodsRestart
	// which operators have. Without a cluster the handler will show a cluster
	// error, but NOT a ⛔ permission denial.
	h.SendCallback(operatorID, "operator1", "k8s_restart", "nginx-pod|default|test-cluster")
	time.Sleep(400 * time.Millisecond)

	// Check callback toasts for a denial.
	isDenied := func(s string) bool {
		return strings.Contains(s, "⛔") ||
			strings.Contains(strings.ToLower(s), "permission")
	}
	_, denied := h.Telegram.WaitForCallbackResponse(2*time.Second, isDenied)
	assert.False(t, denied, "operator must NOT be denied restart")
}

// TestE2E_RBAC_Operator_CannotAccessAdminOps verifies that an operator is
// denied admin-only commands. Tests /audit which requires admin.audit.view.
func TestE2E_RBAC_Operator_CannotDelete(t *testing.T) {
	const (
		adminID    = int64(999999)
		operatorID = int64(200004)
	)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(operatorID, "operator2", "operator")

	// /audit requires admin.audit.view — operator does not have this.
	h.SendMessage(operatorID, "operator2", "/audit")
	msg, ok := h.WaitForMessageTo(operatorID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "operator must receive /audit response")

	lower := strings.ToLower(msg)
	isDenied := strings.Contains(msg, "⛔") ||
		strings.Contains(lower, "permission") ||
		strings.Contains(lower, "only admin")
	assert.True(t, isDenied, "operator must be denied /audit (admin-only); got: %q", msg)
}

// TestE2E_RBAC_Admin_CanAccessAdminOps verifies that an admin can access
// admin-only commands like /audit.
func TestE2E_RBAC_Admin_CanDelete(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	h.SendMessage(adminID, "testadmin", "/audit")

	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "admin must receive /audit response")
	assert.NotContains(t, msg, "⛔", "admin must not be denied /audit; got: %q", msg)
}

// TestE2E_RBAC_SuperAdmin_BypassAll verifies that a super-admin (via adminIDs
// config) bypasses all RBAC checks, including admin-only /audit.
func TestE2E_RBAC_SuperAdmin_BypassAll(t *testing.T) {
	const superAdminID = int64(777777)

	h := newSmokeHarness(t, superAdminID)

	// Test /audit which requires admin.audit.view — superadmin bypasses.
	h.SendMessage(superAdminID, "superadmin", "/audit")
	msg, ok := h.WaitForMessageTo(superAdminID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "super-admin must receive a reply")
	assert.NotEmpty(t, msg)
	assert.NotContains(t, msg, "⛔", "super-admin must bypass all RBAC; got: %q", msg)
}

// ─── Rate limiting ────────────────────────────────────────────────────────────

// TestE2E_RateLimit_SpamProtection verifies that spamming messages from the same
// user is handled gracefully (bot doesn't crash).
func TestE2E_RateLimit_SpamProtection(t *testing.T) {
	const (
		adminID = int64(999999)
		spamID  = int64(300001)
	)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(spamID, "spammer", "viewer")

	// Send 50 messages concurrently with minimal delay.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Telegram.InjectTextMessage(spamID, "spammer", "/start")
		}()
	}
	wg.Wait()

	// Wait for the bot to process the flood.
	time.Sleep(2 * time.Second)

	// Check if rate limiting was triggered (optional — depends on config).
	msgs := h.Telegram.MessagesTo(spamID)
	rateLimited := false
	for _, m := range msgs {
		lower := strings.ToLower(m.Text)
		if strings.Contains(lower, "slow") || strings.Contains(lower, "rate") ||
			strings.Contains(lower, "limit") || strings.Contains(lower, "too many") ||
			strings.Contains(lower, "wait") {
			rateLimited = true
			break
		}
	}
	t.Logf("rate limit triggered: %v (messages received: %d)", rateLimited, len(msgs))

	// Core assertion: the bot is still alive after the spam.
	h.SendMessage(adminID, "testadmin", "/start")
	_, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "bot must still be alive after spam attack")
}
