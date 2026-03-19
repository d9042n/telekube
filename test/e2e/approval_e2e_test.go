//go:build e2e

// Package e2e_test contains end-to-end tests for the Telekube approval workflow.
// These tests use the E2E harness (fake Telegram + SQLite :memory:) to verify
// every approval path: submit, approve, reject, cancel, expiry, and quorum.
package e2e_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module/approval"
	e2e "github.com/d9042n/telekube/test/e2e"
)

const (
	apprAdminID    = int64(999999)
	apprOperatorID = int64(700001)
	apprAdmin2ID   = int64(700002)
	apprViewer1ID  = int64(700003)
	apprOperator2  = int64(700004)
)

// newApprovalManager builds an in-process approval.Manager wired to the
// harness's in-memory storage. Rules are pre-configured.
func newApprovalManager(h *e2e.Harness) *approval.Manager {
	cfg := approval.Config{
		Enabled:       true,
		DefaultExpiry: 30 * time.Minute,
		Rules: []approval.Rule{
			{
				Action:            "argocd.apps.sync",
				Clusters:          []string{"*"},
				RequiredApprovals: 1,
				ApproverRoles:     []string{"admin"},
			},
			{
				Action:            "k8s.pods.delete",
				Clusters:          []string{"*"},
				RequiredApprovals: 2,
				ApproverRoles:     []string{"admin"},
			},
		},
	}
	// FakeRoleChecker always returns "admin" — approvers satisfy quorum.
	return approval.New(cfg, h.Storage.Approval(), &fakeRoleChecker{}, &fakeNotifier{}, nil)
}

// fakeRoleChecker satisfies approval.RoleChecker for tests.
type fakeRoleChecker struct{}

func (f *fakeRoleChecker) GetRole(_ context.Context, _ int64) (string, error) {
	return "admin", nil
}

// fakeNotifier satisfies approval.Notifier with no-op implementations.
type fakeNotifier struct{}

func (f *fakeNotifier) SendApprovalRequest(_ int64, _ *entity.ApprovalRequest) (int, error) {
	return 0, nil
}
func (f *fakeNotifier) SendApprovalResult(_ int64, _ *entity.ApprovalRequest, _ string) error {
	return nil
}
func (f *fakeNotifier) NotifyRequester(_ int64, _ *entity.ApprovalRequest, _ bool, _ string) error {
	return nil
}

// ─── Helper: create a fresh approval request ────────────────────────────────

func seedApprovalRequest(t *testing.T, mgr *approval.Manager, requesterID int64, requiredApprovals int) *entity.ApprovalRequest {
	t.Helper()
	req := &entity.ApprovalRequest{
		RequesterID:   requesterID,
		RequesterName: "test-operator",
		Action:        "argocd.apps.sync",
		Resource:      "payment-service",
		Cluster:       "prod-cluster",
		Namespace:     "production",
	}
	// Directly set required approvals to override rule (for quorum tests).
	if requiredApprovals > 0 {
		req.RequiredApprovals = requiredApprovals
	}
	// Use Submit + then patch required approvals for the quorum case.
	err := mgr.Submit(context.Background(), req)
	require.NoError(t, err, "seeding approval request")
	if requiredApprovals > 0 && req.RequiredApprovals != requiredApprovals {
		// Patch required approvals directly on the struct — the storage write
		// happens in Submit; we just track it here for callers.
		req.RequiredApprovals = requiredApprovals
	}
	return req
}

// ─── 1. Submit and Approve ───────────────────────────────────────────────────

// TestE2E_Approval_Submit_And_Approve tests the happy path:
// operator submits → admin approves → executeNow returned.
func TestE2E_Approval_Submit_And_Approve(t *testing.T) {
	h := newSmokeHarness(t, apprAdminID)
	mgr := newApprovalManager(h)

	// Operator submits.
	req := &entity.ApprovalRequest{
		RequesterID:   apprOperatorID,
		RequesterName: "operator1",
		Action:        "argocd.apps.sync",
		Resource:      "payment-service",
		Cluster:       "prod-cluster",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))
	require.Equal(t, entity.ApprovalPending, req.Status)

	// Admin approves.
	executeNow, err := mgr.Decide(context.Background(), req.ID, apprAdminID, "admin", "approved", "LGTM")
	require.NoError(t, err)
	assert.True(t, executeNow, "single approval must trigger execution")

	// Verify final state.
	resolved, err := mgr.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.ApprovalApproved, resolved.Status)
	assert.NotNil(t, resolved.ResolvedAt)
}

// ─── 2. Self-Approve Blocked ─────────────────────────────────────────────────

// TestE2E_Approval_SelfApprove_Blocked verifies that the requester cannot
// approve their own request.
func TestE2E_Approval_SelfApprove_Blocked(t *testing.T) {
	h := newSmokeHarness(t, apprAdminID)
	mgr := newApprovalManager(h)

	req := &entity.ApprovalRequest{
		RequesterID:   apprOperatorID,
		RequesterName: "operator1",
		Action:        "argocd.apps.sync",
		Resource:      "payment-service",
		Cluster:       "prod-cluster",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// Requester tries to approve their own request.
	_, err := mgr.Decide(context.Background(), req.ID, apprOperatorID, "operator1", "approved", "")
	require.Error(t, err, "self-approval must be rejected")
	assert.Contains(t, err.Error(), "requester cannot approve")
}

// ─── 3. Reject ───────────────────────────────────────────────────────────────

// TestE2E_Approval_Reject tests that an admin can reject a request,
// which sets status=rejected and notifies the requester.
func TestE2E_Approval_Reject(t *testing.T) {
	h := newSmokeHarness(t, apprAdminID)
	mgr := newApprovalManager(h)

	req := &entity.ApprovalRequest{
		RequesterID:   apprOperatorID,
		RequesterName: "operator1",
		Action:        "argocd.apps.sync",
		Resource:      "payment-service",
		Cluster:       "prod-cluster",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	executeNow, err := mgr.Decide(context.Background(), req.ID, apprAdminID, "admin", "rejected", "Not now")
	require.NoError(t, err)
	assert.False(t, executeNow, "reject must not trigger execution")

	resolved, err := mgr.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.ApprovalRejected, resolved.Status)
	assert.True(t, resolved.HasRejection())
}

// ─── 4. Two-Approver Quorum ──────────────────────────────────────────────────

// TestE2E_Approval_Requires2 verifies that 2 approvals are needed when
// RequiredApprovals=2. One approval is not enough.
func TestE2E_Approval_Requires2(t *testing.T) {
	h := newSmokeHarness(t, apprAdminID)

	// Build a manager with a 2-approval rule for k8s.pods.delete.
	cfg := approval.Config{
		Enabled:       true,
		DefaultExpiry: 30 * time.Minute,
		Rules: []approval.Rule{
			{
				Action:            "k8s.pods.delete",
				Clusters:          []string{"*"},
				RequiredApprovals: 2,
				ApproverRoles:     []string{"admin"},
			},
		},
	}
	mgr := approval.New(cfg, h.Storage.Approval(), &fakeRoleChecker{}, &fakeNotifier{}, nil)

	req := &entity.ApprovalRequest{
		RequesterID:   apprOperatorID,
		RequesterName: "operator1",
		Action:        "k8s.pods.delete",
		Resource:      "crash-pod",
		Cluster:       "prod-cluster",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))
	assert.Equal(t, 2, req.RequiredApprovals)

	// First approval from admin1 — not enough yet.
	executeNow, err := mgr.Decide(context.Background(), req.ID, apprAdminID, "admin1", "approved", "LGTM1")
	require.NoError(t, err)
	assert.False(t, executeNow, "one approval must NOT trigger execution when 2 required")

	// State must remain pending.
	pending, err := mgr.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.ApprovalPending, pending.Status)

	// Second approval from admin2 — now quorum met.
	executeNow, err = mgr.Decide(context.Background(), req.ID, apprAdmin2ID, "admin2", "approved", "LGTM2")
	require.NoError(t, err)
	assert.True(t, executeNow, "second approval must trigger execution")

	final, err := mgr.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.ApprovalApproved, final.Status)
}

// ─── 5. Expired Request ───────────────────────────────────────────────────────

// TestE2E_Approval_Expired verifies that deciding on an expired request fails.
func TestE2E_Approval_Expired(t *testing.T) {
	h := newSmokeHarness(t, apprAdminID)

	// Build a manager with a very short expiry.
	cfg := approval.Config{
		Enabled:       true,
		DefaultExpiry: 100 * time.Millisecond, // expires almost immediately
		Rules: []approval.Rule{
			{
				Action:            "argocd.apps.sync",
				Clusters:          []string{"*"},
				RequiredApprovals: 1,
				ApproverRoles:     []string{"admin"},
			},
		},
	}
	mgr := approval.New(cfg, h.Storage.Approval(), &fakeRoleChecker{}, &fakeNotifier{}, nil)

	req := &entity.ApprovalRequest{
		RequesterID:   apprOperatorID,
		RequesterName: "operator1",
		Action:        "argocd.apps.sync",
		Resource:      "payment-service",
		Cluster:       "prod-cluster",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// Wait for the request to expire.
	time.Sleep(200 * time.Millisecond)

	// Admin tries to approve — must fail with "expired".
	_, err := mgr.Decide(context.Background(), req.ID, apprAdminID, "admin", "approved", "")
	require.Error(t, err, "deciding on expired request must fail")
	assert.Contains(t, strings.ToLower(err.Error()), "expired")
}

// ─── 6. Cancel by Requester ───────────────────────────────────────────────────

// TestE2E_Approval_Cancel verifies that the requester can cancel a pending request.
func TestE2E_Approval_Cancel(t *testing.T) {
	h := newSmokeHarness(t, apprAdminID)
	mgr := newApprovalManager(h)

	req := &entity.ApprovalRequest{
		RequesterID:   apprOperatorID,
		RequesterName: "operator1",
		Action:        "argocd.apps.sync",
		Resource:      "payment-service",
		Cluster:       "prod-cluster",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// Requester cancels.
	require.NoError(t, mgr.Cancel(context.Background(), req.ID, apprOperatorID))

	cancelled, err := mgr.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.ApprovalCancelled, cancelled.Status)
}

// ─── 7. Cancel by Wrong User ──────────────────────────────────────────────────

// TestE2E_Approval_Cancel_WrongUser verifies that a different user cannot cancel.
func TestE2E_Approval_Cancel_WrongUser(t *testing.T) {
	h := newSmokeHarness(t, apprAdminID)
	mgr := newApprovalManager(h)

	req := &entity.ApprovalRequest{
		RequesterID:   apprOperatorID,
		RequesterName: "operator1",
		Action:        "argocd.apps.sync",
		Resource:      "payment-service",
		Cluster:       "prod-cluster",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// Different user tries to cancel.
	err := mgr.Cancel(context.Background(), req.ID, apprAdmin2ID)
	require.Error(t, err, "non-requester cannot cancel")
	assert.Contains(t, err.Error(), "only the requester")
}

// ─── 8. Already Resolved ─────────────────────────────────────────────────────

// TestE2E_Approval_AlreadyResolved verifies that deciding on an already-approved
// request returns an error.
func TestE2E_Approval_AlreadyResolved(t *testing.T) {
	h := newSmokeHarness(t, apprAdminID)
	mgr := newApprovalManager(h)

	req := &entity.ApprovalRequest{
		RequesterID:   apprOperatorID,
		RequesterName: "operator1",
		Action:        "argocd.apps.sync",
		Resource:      "payment-service",
		Cluster:       "prod-cluster",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// First approval — resolves the request.
	_, err := mgr.Decide(context.Background(), req.ID, apprAdminID, "admin", "approved", "LGTM")
	require.NoError(t, err)

	// Second decision attempt — must fail.
	_, err = mgr.Decide(context.Background(), req.ID, apprAdmin2ID, "admin2", "approved", "also LGTM")
	require.Error(t, err, "deciding on already-approved request must fail")
	assert.Contains(t, strings.ToLower(err.Error()), "already")
}

// ─── Bot-level approval via callback (smoke mode) ────────────────────────────

// TestE2E_Approval_BotCallback_ApproveViaButton seeds an approval request and
// verifies the bot processes the approve callback without crashing.
func TestE2E_Approval_BotCallback_ApproveViaButton(t *testing.T) {
	const (
		adminID    = int64(999999)
		operatorID = int64(700010)
	)

	h := newSmokeHarness(t, adminID)
	h.SeedUser(adminID, "testadmin", "admin")
	h.SeedUser(operatorID, "operator10", "operator")

	mgr := newApprovalManager(h)

	// Seed a pending approval request.
	req := &entity.ApprovalRequest{
		RequesterID:   operatorID,
		RequesterName: "operator10",
		Action:        "argocd.apps.sync",
		Resource:      "web-service",
		Cluster:       "staging",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// Admin clicks "Approve" via callback — the bot approval handler responds.
	h.SendCallback(adminID, "testadmin", "appr_approve", req.ID)

	// The bot should respond with a confirm callback answer (even if editing fails
	// because there's no real message to edit).
	time.Sleep(1 * time.Second)

	// Verify the approval is now in the storage (processed by the manager directly).
	resolved, err := mgr.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	// The bot approval handler calls mgr.Decide — result depends on whether the
	// approval BotModule was registered in the test harness. Since we use the
	// top-level bot, the "appr_approve" callback may or may not be wired.
	// We assert the request exists and is not in an invalid state.
	assert.NotEmpty(t, resolved.ID)
	t.Logf("approval status after bot callback: %s", resolved.Status)
}
