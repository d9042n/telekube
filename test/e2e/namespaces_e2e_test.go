//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2e "github.com/d9042n/telekube/test/e2e"
)

const namespacesAdminID = int64(999999)

func TestE2E_Namespaces_Smoke(t *testing.T) {
	h := newSmokeHarness(t, namespacesAdminID)
	h.SeedUser(namespacesAdminID, "testadmin", "admin")

	h.SendMessage(namespacesAdminID, "testadmin", "/namespaces")
	msg, ok := h.WaitForMessageTo(namespacesAdminID, 5*time.Second, func(s string) bool {
		return s != ""
	})
	require.True(t, ok, "bot must reply to /namespaces")
	assert.NotContains(t, strings.ToLower(msg), "unknown command")
}

func TestE2E_Namespaces_WithK3s(t *testing.T) {
	if e2e.IsSkipCluster() {
		t.Skip("E2E_SKIP_CLUSTER=true")
	}

	h := e2e.NewHarness(t, e2e.WithAdminIDs(namespacesAdminID))
	h.SeedUser(namespacesAdminID, "testadmin", "admin")

	h.SendMessage(namespacesAdminID, "testadmin", "/namespaces")
	msg, ok := h.WaitForMessageTo(namespacesAdminID, 10*time.Second, func(s string) bool {
		return strings.Contains(s, "Namespaces")
	})
	require.True(t, ok, "bot must reply with namespace list")
	assert.Contains(t, msg, "default")
	assert.Contains(t, msg, "kube-system")
}
