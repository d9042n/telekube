package approval_test

import (
	"context"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module/approval"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── In-memory approval repository ──────────────────────────────────────────

type memApprovalRepo struct {
	records map[string]*entity.ApprovalRequest
}

func newMemApprovalRepo() *memApprovalRepo {
	return &memApprovalRepo{records: make(map[string]*entity.ApprovalRequest)}
}

func (r *memApprovalRepo) Create(_ context.Context, req *entity.ApprovalRequest) error {
	r.records[req.ID] = req
	return nil
}

func (r *memApprovalRepo) GetByID(_ context.Context, id string) (*entity.ApprovalRequest, error) {
	req, ok := r.records[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return req, nil
}

func (r *memApprovalRepo) Update(_ context.Context, req *entity.ApprovalRequest) error {
	if _, ok := r.records[req.ID]; !ok {
		return storage.ErrNotFound
	}
	r.records[req.ID] = req
	return nil
}

func (r *memApprovalRepo) ListPending(_ context.Context) ([]entity.ApprovalRequest, error) {
	var out []entity.ApprovalRequest
	for _, req := range r.records {
		if req.Status == entity.ApprovalPending {
			out = append(out, *req)
		}
	}
	return out, nil
}

func (r *memApprovalRepo) ListByRequester(_ context.Context, userID int64) ([]entity.ApprovalRequest, error) {
	var out []entity.ApprovalRequest
	for _, req := range r.records {
		if req.RequesterID == userID {
			out = append(out, *req)
		}
	}
	return out, nil
}

func (r *memApprovalRepo) ExpireOld(_ context.Context) (int, error) {
	n := 0
	now := time.Now()
	for id, req := range r.records {
		if req.Status == entity.ApprovalPending && req.ExpiresAt.Before(now) {
			req.Status = entity.ApprovalExpired
			r.records[id] = req
			n++
		}
	}
	return n, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func defaultConfig() approval.Config {
	return approval.Config{
		Enabled:       true,
		DefaultExpiry: 30 * time.Minute,
		Rules: []approval.Rule{
			{
				Action:            "argocd.apps.sync",
				Clusters:          []string{"prod-1"},
				RequiredApprovals: 1,
				ApproverRoles:     []string{"admin"},
			},
			{
				Action:            "kubernetes.nodes.drain",
				Clusters:          []string{"*"},
				RequiredApprovals: 2,
				ApproverRoles:     []string{"admin"},
			},
		},
	}
}

func newManager(cfg approval.Config) (*approval.Manager, *memApprovalRepo) {
	repo := newMemApprovalRepo()
	mgr := approval.New(cfg, repo, nil, nil, nil)
	return mgr, repo
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestManager_RequiresApproval_MatchesRule(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	rule := mgr.RequiresApproval("argocd.apps.sync", "prod-1")
	require.NotNil(t, rule)
	assert.Equal(t, 1, rule.RequiredApprovals)
}

func TestManager_RequiresApproval_WildcardCluster(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	rule := mgr.RequiresApproval("kubernetes.nodes.drain", "any-cluster")
	require.NotNil(t, rule)
	assert.Equal(t, 2, rule.RequiredApprovals)
}

func TestManager_RequiresApproval_NoMatch(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	rule := mgr.RequiresApproval("argocd.apps.sync", "staging") // staging not in rules
	assert.Nil(t, rule)
}

func TestManager_RequiresApproval_Disabled(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	cfg.Enabled = false
	mgr, _ := newManager(cfg)

	rule := mgr.RequiresApproval("argocd.apps.sync", "prod-1")
	assert.Nil(t, rule)
}

func TestManager_Submit(t *testing.T) {
	t.Parallel()
	mgr, repo := newManager(defaultConfig())

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Resource:      "my-app",
		Cluster:       "prod-1",
	}

	err := mgr.Submit(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, req.ID)
	assert.Equal(t, entity.ApprovalPending, req.Status)
	assert.Equal(t, 1, req.RequiredApprovals)

	stored, err := repo.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, req.ID, stored.ID)
}

func TestManager_Submit_NoMatchingRule(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	req := &entity.ApprovalRequest{
		RequesterID: 100,
		Action:      "argocd.apps.sync",
		Cluster:     "dev", // no rule for dev
	}

	err := mgr.Submit(context.Background(), req)
	assert.Error(t, err)
}

func TestManager_Decide_Approve(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	executeNow, err := mgr.Decide(context.Background(), req.ID, 200, "bob", "approved", "LGTM")
	require.NoError(t, err)
	assert.True(t, executeNow)

	fetched, err := mgr.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.ApprovalApproved, fetched.Status)
}

func TestManager_Decide_Reject(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	executeNow, err := mgr.Decide(context.Background(), req.ID, 200, "bob", "rejected", "No")
	require.NoError(t, err)
	assert.False(t, executeNow)

	fetched, err := mgr.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.ApprovalRejected, fetched.Status)
}

func TestManager_Decide_SelfApproval_Denied(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	_, err := mgr.Decide(context.Background(), req.ID, 100, "alice", "approved", "self")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requester cannot approve")
}

func TestManager_Decide_RequiresTwoApprovals(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	// kubernetes.nodes.drain requires 2 approvals.
	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "kubernetes.nodes.drain",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// First approval should NOT execute yet.
	execute, err := mgr.Decide(context.Background(), req.ID, 200, "bob", "approved", "ok")
	require.NoError(t, err)
	assert.False(t, execute)

	// Second approval should trigger execution.
	execute, err = mgr.Decide(context.Background(), req.ID, 300, "carol", "approved", "ok")
	require.NoError(t, err)
	assert.True(t, execute)
}

func TestManager_Cancel(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	require.NoError(t, mgr.Cancel(context.Background(), req.ID, 100))

	fetched, err := mgr.GetByID(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.ApprovalCancelled, fetched.Status)
}

func TestManager_Cancel_WrongUser(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	err := mgr.Cancel(context.Background(), req.ID, 999)
	assert.Error(t, err)
}

func TestManager_ListPending(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	for i := 0; i < 3; i++ {
		req := &entity.ApprovalRequest{
			RequesterID: int64(100 + i),
			Action:      "argocd.apps.sync",
			Cluster:     "prod-1",
		}
		require.NoError(t, mgr.Submit(context.Background(), req))
	}

	pending, err := mgr.ListPending(context.Background())
	require.NoError(t, err)
	assert.Len(t, pending, 3)
}

func TestManager_Decide_AlreadyResolved(t *testing.T) {
	t.Parallel()
	mgr, _ := newManager(defaultConfig())

	req := &entity.ApprovalRequest{
		RequesterID:   100,
		RequesterName: "alice",
		Action:        "argocd.apps.sync",
		Cluster:       "prod-1",
	}
	require.NoError(t, mgr.Submit(context.Background(), req))

	// First approve — resolves it.
	_, err := mgr.Decide(context.Background(), req.ID, 200, "bob", "approved", "ok")
	require.NoError(t, err)

	// Second attempt should fail.
	_, err = mgr.Decide(context.Background(), req.ID, 300, "carol", "approved", "late")
	assert.Error(t, err)
}
