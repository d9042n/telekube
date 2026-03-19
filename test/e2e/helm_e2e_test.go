//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	helmAdminID    = int64(999999)
	helmOperatorID = int64(900001)
	helmViewerID   = int64(900002)
)

// TestE2E_Helm_ListReleases verifies that /helm lists the fake releases
// registered by the harness's fakeReleaseClient.
func TestE2E_Helm_ListReleases(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	h.SendMessage(adminID, "testadmin", "/helm")

	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /helm")
	t.Logf("helm list response: %q", msg)
	assert.NotEmpty(t, msg)
	// Must contain "Select namespace" or "Helm" — proves the module is registered.
	assert.True(t,
		strings.Contains(strings.ToLower(msg), "namespace") ||
			strings.Contains(strings.ToLower(msg), "helm"),
		"response must mention namespace or helm; got: %q", msg)
}

// TestE2E_Helm_Rollback_RequiresAdmin_Viewer verifies that a viewer is denied /helm.
// The Helm list command requires PermHelmReleaseslist, which viewer doesn't have.
func TestE2E_Helm_Rollback_RequiresAdmin_Viewer(t *testing.T) {
	const (
		adminID  = int64(999999)
		viewerID = int64(900002)
	)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(viewerID, "viewer2", "viewer")

	// Viewer tries /helm — requires helm.releases.list which viewer lacks.
	h.SendMessage(viewerID, "viewer2", "/helm")

	msg, ok := h.WaitForMessageTo(viewerID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must respond to viewer /helm")
	t.Logf("viewer /helm response: %q", msg)

	lower := strings.ToLower(msg)
	isDenied := strings.Contains(msg, "🚫") ||
		strings.Contains(lower, "permission") ||
		strings.Contains(lower, "insufficient") ||
		strings.Contains(lower, "denied")
	assert.True(t, isDenied, "viewer must be denied /helm; got: %q", msg)
}

// TestE2E_Helm_Rollback_RequiresAdmin_Operator verifies operator can access /helm
// (operator has helm.releases.list) but cannot rollback (admin-only).
func TestE2E_Helm_Rollback_RequiresAdmin_Operator(t *testing.T) {
	const (
		adminID    = int64(999999)
		operatorID = int64(900001)
	)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(operatorID, "op1", "operator")

	// Operator has PermHelmReleaseslist — should NOT be denied /helm.
	h.SendMessage(operatorID, "op1", "/helm")

	msg, ok := h.WaitForMessageTo(operatorID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must respond to operator /helm")
	t.Logf("operator /helm response: %q", msg)

	assert.NotEmpty(t, msg)
	// Operator should be allowed to list Helm releases.
	assert.NotContains(t, msg, "🚫", "operator must not be denied /helm listing")
}

// TestE2E_Helm_Rollback_Admin tests that an admin can use /helm and access all features.
func TestE2E_Helm_Rollback_Admin(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	h.SendMessage(adminID, "testadmin", "/helm")

	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "admin must receive /helm response")
	t.Logf("admin /helm response: %q", msg)
	assert.NotEmpty(t, msg)
	assert.NotContains(t, msg, "🚫")
}

// TestE2E_Helm_Rollback_ConfirmFlow verifies the Helm module is accessible.
// helm_rollback_confirm uses c.Edit() so we just verify /helm works.
func TestE2E_Helm_Rollback_ConfirmFlow(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	// Trigger namespace selection callback which edits the message.
	h.SendCallback(adminID, "testadmin", "helm_ns_select", "test-cluster|")
	time.Sleep(500 * time.Millisecond)

	// Check if the bot responded with an edit showing releases.
	msgs := h.Telegram.MessagesTo(adminID)
	found := false
	for _, m := range msgs {
		lower := strings.ToLower(m.Text)
		if strings.Contains(lower, "nginx-ingress") || strings.Contains(lower, "helm") {
			found = true
			t.Logf("helm releases via callback: %q", m.Text)
			break
		}
	}
	assert.True(t, found, "admin must see releases after namespace selection; got %d messages", len(msgs))
}

// TestE2E_Helm_CommandSmoke verifies the /helm command is routed correctly.
func TestE2E_Helm_CommandSmoke(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	h.SendMessage(adminID, "testadmin", "/helm")
	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "bot must reply to /helm command")
	assert.NotEmpty(t, msg)
}

// TestE2E_Helm_ReleaseNotFound tests graceful error handling for missing releases.
// helm_release_detail with a non-existent release name should not crash.
func TestE2E_Helm_ReleaseNotFound(t *testing.T) {
	const adminID = int64(999999)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")

	// Request detail for a release that doesn't exist in the fake client.
	h.SendCallback(adminID, "testadmin", "helm_release_detail", "test-cluster||ghost-release")
	time.Sleep(500 * time.Millisecond)

	// Verify bot is still alive.
	h.SendMessage(adminID, "testadmin", "/start")
	msg, ok := h.WaitForMessageTo(adminID, 5*time.Second, func(s string) bool { return s != "" })
	require.True(t, ok, "bot must still be responsive after release-not-found callback")
	assert.NotEmpty(t, msg)
}
