package handler_test

import (
	"context"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/bot/handler"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ─── Mock audit.Logger ────────────────────────────────────────────────────────

type mockAuditLogger struct{ mock.Mock }

func (m *mockAuditLogger) Log(e entity.AuditEntry) { m.Called(e) }
func (m *mockAuditLogger) Query(ctx context.Context, filter storage.AuditFilter) ([]entity.AuditEntry, int, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]entity.AuditEntry), args.Int(1), args.Error(2)
}
func (m *mockAuditLogger) Flush(_ context.Context) error { return nil }
func (m *mockAuditLogger) Close() error                 { return nil }

// ─── AuditLog handler tests ───────────────────────────────────────────────────

func TestAuditLog_NoUser_SendsWarning(t *testing.T) {
	t.Parallel()

	logger := &mockAuditLogger{}
	rb := newAllowRBAC(entity.RoleAdmin)

	ctx := newFakeCtx(nil)
	h := handler.AuditLog(logger, rb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Contains(t, ctx.messages[0], "Could not identify you")
}

func TestAuditLog_NoPermission_SendsDenied(t *testing.T) {
	t.Parallel()

	logger := &mockAuditLogger{}
	rb := newDenyRBAC()

	user := testUser(1, "alice", entity.RoleViewer)
	ctx := newFakeCtx(user)

	h := handler.AuditLog(logger, rb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Contains(t, ctx.messages[0], "don't have permission")
}

func TestAuditLog_EmptyLog_ShowsNoEntries(t *testing.T) {
	t.Parallel()

	logger := &mockAuditLogger{}
	logger.On("Query", mock.Anything, mock.Anything).
		Return([]entity.AuditEntry{}, 0, nil)

	rb := newAllowRBAC(entity.RoleAdmin)

	user := testUser(1, "admin", entity.RoleAdmin)
	ctx := newFakeCtx(user)

	h := handler.AuditLog(logger, rb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Contains(t, ctx.messages[0], "No actions recorded")
}

func TestAuditLog_WithEntries_ShowsFormatted(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	entries := []entity.AuditEntry{
		{
			UserID:     1,
			Username:   "alice",
			Action:     "kubernetes.pods.restart",
			Status:     entity.AuditStatusSuccess,
			Cluster:    "prod",
			OccurredAt: now,
		},
		{
			UserID:     2,
			Username:   "",
			Action:     "argocd.app.sync",
			Status:     entity.AuditStatusError,
			Cluster:    "",
			OccurredAt: now,
		},
	}

	logger := &mockAuditLogger{}
	logger.On("Query", mock.Anything, mock.Anything).
		Return(entries, 2, nil)

	rb := newAllowRBAC(entity.RoleAdmin)

	user := testUser(1, "admin", entity.RoleAdmin)
	ctx := newFakeCtx(user)

	h := handler.AuditLog(logger, rb)
	err := h(ctx)

	assert.NoError(t, err)
	msg := ctx.messages[0]
	assert.Contains(t, msg, "alice")
	assert.Contains(t, msg, "prod")
	assert.Contains(t, msg, "kubernetes.pods.restart")

	// Empty username should fallback to id:<UserID>
	assert.Contains(t, msg, "id:2")

	// Summary shows total count
	assert.Contains(t, msg, "Showing 2 of 2")
}

func TestAuditLog_LongAction_Truncated(t *testing.T) {
	t.Parallel()

	longAction := "this.is.a.very.long.action.name.that.exceeds.forty.characters.really.long"
	entries := []entity.AuditEntry{
		{
			UserID:     1,
			Username:   "bob",
			Action:     longAction,
			Status:     entity.AuditStatusSuccess,
			OccurredAt: time.Now().UTC(),
		},
	}

	logger := &mockAuditLogger{}
	logger.On("Query", mock.Anything, mock.Anything).
		Return(entries, 1, nil)

	rb := newAllowRBAC(entity.RoleAdmin)

	user := testUser(1, "admin", entity.RoleAdmin)
	ctx := newFakeCtx(user)

	h := handler.AuditLog(logger, rb)
	err := h(ctx)

	assert.NoError(t, err)
	// Full action (73 chars) should be truncated to 40 + "..."
	assert.Contains(t, ctx.messages[0], "...")
}

func TestAuditLog_QueryError_ShowsError(t *testing.T) {
	t.Parallel()

	logger := &mockAuditLogger{}
	logger.On("Query", mock.Anything, mock.Anything).
		Return([]entity.AuditEntry{}, 0, assert.AnError)

	rb := newAllowRBAC(entity.RoleAdmin)

	user := testUser(1, "admin", entity.RoleAdmin)
	ctx := newFakeCtx(user)

	h := handler.AuditLog(logger, rb)
	err := h(ctx)

	assert.NoError(t, err) // handler returns nil, sends error message
	assert.Contains(t, ctx.messages[0], "Failed to load audit log")
}

// ─── actionEmoji and statusLabel (exported via export_test.go) ───────────────
// Since these are package-level private functions we test them transitively
// via AuditLog — the emoji/label appears in the formatted output.

func TestActionEmoji_SuccessAndErrorPathsInOutput(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	entries := []entity.AuditEntry{
		{Username: "u1", Action: "a", Status: entity.AuditStatusSuccess, OccurredAt: now},
		{Username: "u2", Action: "b", Status: entity.AuditStatusError, OccurredAt: now},
		{Username: "u3", Action: "c", Status: entity.AuditStatusDenied, OccurredAt: now},
		{Username: "u4", Action: "d", Status: "", OccurredAt: now},
	}

	logger := &mockAuditLogger{}
	logger.On("Query", mock.Anything, mock.Anything).
		Return(entries, 4, nil)

	rb := newAllowRBAC(entity.RoleAdmin)
	user := testUser(1, "admin", entity.RoleAdmin)
	ctx := newFakeCtx(user)

	h := handler.AuditLog(logger, rb)
	_ = h(ctx)

	msg := ctx.messages[0]
	assert.Contains(t, msg, "✅")  // success emoji
	assert.Contains(t, msg, "❌")  // error emoji
	assert.Contains(t, msg, "⛔")  // denied emoji
	assert.Contains(t, msg, "ℹ️") // default emoji for empty status
}
