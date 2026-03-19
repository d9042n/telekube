package approval_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module/approval"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Expired request: Decide should return "expired" immediately ──────────────

func TestManager_Decide_ExpiredRequest_ReturnsError(t *testing.T) {
	t.Parallel()

	cfg := defaultConfig()
	// Give the request a very short expiry.
	cfg.DefaultExpiry = time.Millisecond
	mgr, _ := newManager(cfg)

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// Wait for it to expire.
	time.Sleep(50 * time.Millisecond)

	_, err := mgr.Decide(context.Background(), req.ID, 200, "bob", "approved", "LGTM")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

// ─── Same approver votes twice: must override, not double-count ───────────────

func TestManager_Decide_DuplicateApprover_OverridesVote(t *testing.T) {
	t.Parallel()

	// We need 2 approvals to complete.
	cfg := approval.Config{
		Enabled:       true,
		DefaultExpiry: 30 * time.Minute,
		Rules: []approval.Rule{{
			Action:            "argocd.apps.sync",
			Clusters:          []string{"prod-1"},
			RequiredApprovals: 2,
			ApproverRoles:     []string{"admin"},
		}},
	}
	mgr, _ := newManager(cfg)

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// Bob votes approved once.
	execute, err := mgr.Decide(context.Background(), req.ID, 200, "bob", "approved", "first vote")
	require.NoError(t, err)
	assert.False(t, execute, "1 approval → not enough, still pending")

	// Bob votes approved again (override, same userID).
	execute, err = mgr.Decide(context.Background(), req.ID, 200, "bob", "approved", "second vote")
	require.NoError(t, err)
	// ApprovalCount should be 1 (Bob's single vote), not 2.
	assert.False(t, execute, "same approver voting twice must not double-count")

	// Verify state: still pending with 1 approval (Bob's only).
	fetched, err := mgr.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, fetched.ApprovalCount(), "duplicate vote must override, not add")
	assert.Equal(t, entity.ApprovalPending, fetched.Status, "still pending after duplicate vote")
}

// ─── RequiredApprovals = 0: document and assert behavior ─────────────────────

func TestManager_Decide_ZeroRequiredApprovals_Behavior(t *testing.T) {
	t.Parallel()
	// With RequiredApprovals=0, ApprovalCount() >= 0 is always true from the
	// first non-rejection decision. This is a documented edge-case behavior.
	cfg := approval.Config{
		Enabled:       true,
		DefaultExpiry: 30 * time.Minute,
		Rules: []approval.Rule{{
			Action:            "argocd.apps.sync",
			Clusters:          []string{"prod-1"},
			RequiredApprovals: 0, // zero
			ApproverRoles:     []string{"admin"},
		}},
	}
	mgr, _ := newManager(cfg)

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// With required=0: any approval should trigger execution immediately.
	execute, err := mgr.Decide(context.Background(), req.ID, 200, "bob", "approved", "ok")
	require.NoError(t, err)
	assert.True(t, execute, "with RequiredApprovals=0, first approval should trigger execution")
}

// ─── Cancel already-approved request → error "not pending" ───────────────────

func TestManager_Cancel_AlreadyApproved_ReturnsError(t *testing.T) {
	t.Parallel()

	mgr, _ := newManager(defaultConfig())

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// Approve it.
	_, err := mgr.Decide(context.Background(), req.ID, 200, "bob", "approved", "ok")
	require.NoError(t, err)

	// Try to cancel after it's approved.
	err = mgr.Cancel(context.Background(), req.ID, 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not pending")
}

// ─── Decide on non-existent request ID → ErrNotFound ─────────────────────────

func TestManager_Decide_NonExistentID_ReturnsError(t *testing.T) {
	t.Parallel()

	mgr, _ := newManager(defaultConfig())

	_, err := mgr.Decide(context.Background(), "non-existent-id", 200, "bob", "approved", "ok")
	require.Error(t, err)
}

// ─── Concurrent Decide from 2 approvers simultaneously → no race condition ───

func TestManager_Decide_Concurrent_NoRace(t *testing.T) {
	t.Parallel()

	// Need 2 approvals.
	cfg := approval.Config{
		Enabled:       true,
		DefaultExpiry: 30 * time.Minute,
		Rules: []approval.Rule{{
			Action:            "kubernetes.nodes.drain",
			Clusters:          []string{"*"},
			RequiredApprovals: 2,
			ApproverRoles:     []string{"admin"},
		}},
	}
	mgr, _ := newManager(cfg)

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "kubernetes.nodes.drain",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	var wg sync.WaitGroup
	var totalExecutions int64
	var mu sync.Mutex

	for _, approverID := range []int64{200, 300} {
		approverID := approverID
		wg.Add(1)
		go func() {
			defer wg.Done()
			execute, err := mgr.Decide(context.Background(), req.ID, approverID, "approver", "approved", "ok")
			if err == nil && execute {
				mu.Lock()
				totalExecutions++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Exactly one approval should trigger execution (not both).
	assert.Equal(t, int64(1), totalExecutions, "only one Decide call should signal execution=true")
}

// ─── Self-approval rejection ──────────────────────────────────────────────────

func TestManager_Decide_SelfApproval_RequesterIsAlwaysBlocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		decision  string
	}{
		{"self-approve blocked", "approved"},
		// Note: self-reject via Decide might be intentionally allowed or blocked;
		// the current implementation blocks self-approve only.
		// We test the documented behavior.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mgr, _ := newManager(defaultConfig())
			req := &entity.ApprovalRequest{
				RequesterID:   100,
				RequesterName: "alice",
				Action:        "argocd.apps.sync",
				Cluster:       "prod-1",
			}
			require.NoError(t, mgr.Submit(context.Background(), req))

			_, err := mgr.Decide(context.Background(), req.ID, 100, "alice", tt.decision, "self")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "requester cannot approve")
		})
	}
}

// ─── Cancel non-existent ID ───────────────────────────────────────────────────

func TestManager_Cancel_NonExistentID_ReturnsError(t *testing.T) {
	t.Parallel()

	mgr, _ := newManager(defaultConfig())

	err := mgr.Cancel(context.Background(), "does-not-exist", 100)
	require.Error(t, err)
}

// ─── ListPending with no pending requests ─────────────────────────────────────

func TestManager_ListPending_Empty(t *testing.T) {
	t.Parallel()

	mgr, _ := newManager(defaultConfig())

	pending, err := mgr.ListPending(context.Background())
	require.NoError(t, err)
	assert.Empty(t, pending, "empty manager should have no pending requests")
}

// ─── Submit: approval request is disabled ────────────────────────────────────

func TestManager_Submit_WhenDisabled_ReturnsError(t *testing.T) {
	t.Parallel()

	cfg := defaultConfig()
	cfg.Enabled = false
	mgr, _ := newManager(cfg)

	req := &entity.ApprovalRequest{
		RequesterID: 100,
		Action:      "argocd.apps.sync",
		Cluster:     "prod-1",
	}
	err := mgr.Submit(context.Background(), req)
	assert.Error(t, err, "submit should fail when approval is disabled")
}

// ─── Approved then another approve → error ────────────────────────────────────

func TestManager_Decide_AlreadyApproved_SecondDecide_ReturnsError(t *testing.T) {
	t.Parallel()

	mgr, _ := newManager(defaultConfig())

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// First approve resolves it.
	_, err := mgr.Decide(context.Background(), req.ID, 200, "bob", "approved", "ok")
	require.NoError(t, err)

	// Second attempt on already-resolved request.
	_, err = mgr.Decide(context.Background(), req.ID, 300, "carol", "approved", "too late")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already")
}

// ─── Rejected then another decision → error ───────────────────────────────────

func TestManager_Decide_AfterRejection_ReturnsError(t *testing.T) {
	t.Parallel()

	mgr, _ := newManager(defaultConfig())

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// Bob rejects.
	_, err := mgr.Decide(context.Background(), req.ID, 200, "bob", "rejected", "No")
	require.NoError(t, err)

	// Carol tries to approve after rejection resolved the request.
	_, err = mgr.Decide(context.Background(), req.ID, 300, "carol", "approved", "late")
	require.Error(t, err)
}

// ─── GetByID for existing request ─────────────────────────────────────────────

func TestManager_GetByID_Existing(t *testing.T) {
	t.Parallel()

	mgr, _ := newManager(defaultConfig())
	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	fetched, err := mgr.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, req.ID, fetched.ID)
	assert.Equal(t, entity.ApprovalPending, fetched.Status)
}

func TestManager_GetByID_NonExistent_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	mgr, _ := newManager(defaultConfig())
	_, err := mgr.GetByID(context.Background(), "fake-id")
	require.Error(t, err)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}
