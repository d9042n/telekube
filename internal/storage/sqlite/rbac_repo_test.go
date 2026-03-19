package sqlite

// rbac_repo_test.go — full coverage for rbacRepo methods (CreateRole, GetRole,
// ListRoles, DeleteRole, CreateRoleBinding, GetUserRoleBindings,
// DeleteRoleBinding, ListAllBindings) and approvalRepo methods.

import (
	"context"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── rbacRepo.CreateRole / GetRole / ListRoles / DeleteRole ──────────────────

func TestRBACRepo_CreateAndGetRole(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	role := &entity.Role{
		Name:        "deployer",
		DisplayName: "Deployer",
		Description: "Can deploy apps",
		IsBuiltin:   false,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		Rules: []entity.PolicyRule{
			{
				Modules:    []string{"kubernetes"},
				Resources:  []string{"deployments"},
				Actions:    []string{"scale"},
				Clusters:   []string{"*"},
				Namespaces: []string{"*"},
				Effect:     "allow",
			},
		},
	}

	require.NoError(t, store.RBAC().CreateRole(ctx, role))

	got, err := store.RBAC().GetRole(ctx, "deployer")
	require.NoError(t, err)
	assert.Equal(t, "deployer", got.Name)
	assert.Equal(t, "Deployer", got.DisplayName)
	assert.Len(t, got.Rules, 1)
	assert.Equal(t, []string{"kubernetes"}, got.Rules[0].Modules)
}

func TestRBACRepo_GetRole_NotFound(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	_, err := store.RBAC().GetRole(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestRBACRepo_CreateRole_IdempotentUpsert(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	role := &entity.Role{
		Name:        "myrole",
		DisplayName: "Original",
		Rules:       nil,
		CreatedAt:   time.Now().UTC(),
	}
	require.NoError(t, store.RBAC().CreateRole(ctx, role))

	// Upsert with updated display name
	role.DisplayName = "Updated"
	require.NoError(t, store.RBAC().CreateRole(ctx, role))

	got, err := store.RBAC().GetRole(ctx, "myrole")
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.DisplayName, "upsert should overwrite display_name")
}

func TestRBACRepo_ListRoles(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	names := []string{"alpha", "beta", "gamma"}
	for _, n := range names {
		require.NoError(t, store.RBAC().CreateRole(ctx, &entity.Role{
			Name:        n,
			DisplayName: n,
			CreatedAt:   time.Now().UTC(),
		}))
	}

	roles, err := store.RBAC().ListRoles(ctx)
	require.NoError(t, err)
	assert.Len(t, roles, 3)
}

func TestRBACRepo_DeleteRole(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.RBAC().CreateRole(ctx, &entity.Role{
		Name:      "todelete",
		CreatedAt: time.Now().UTC(),
	}))

	require.NoError(t, store.RBAC().DeleteRole(ctx, "todelete"))

	_, err := store.RBAC().GetRole(ctx, "todelete")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

// ─── rbacRepo.SetUserRole — user not found path ──────────────────────────────

func TestRBACRepo_SetUserRole_UserNotFound(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Setting role on non-existent user should return ErrNotFound (0 rows affected)
	err := store.RBAC().SetUserRole(context.Background(), 99999, entity.RoleAdmin)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

// ─── rbacRepo.CreateRoleBinding / GetUserRoleBindings / DeleteRoleBinding ────

func TestRBACRepo_RoleBindingLifecycle(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// FK: role must exist first
	require.NoError(t, store.RBAC().CreateRole(ctx, &entity.Role{
		Name: "deployer", DisplayName: "Deployer", CreatedAt: time.Now().UTC(),
	}))
	// FK: user must exist first
	require.NoError(t, store.Users().Upsert(ctx, &entity.User{
		TelegramID: 42, Username: "binduser", Role: entity.RoleViewer, IsActive: true,
	}))

	expiry := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	b := &entity.UserRoleBinding{
		UserID:    42,
		RoleName:  "deployer",
		ExpiresAt: &expiry,
	}
	require.NoError(t, store.RBAC().CreateRoleBinding(ctx, b))
	assert.NotEmpty(t, b.ID, "ID should be auto-generated")

	bindings, err := store.RBAC().GetUserRoleBindings(ctx, 42)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	assert.Equal(t, "deployer", bindings[0].RoleName)
	assert.NotNil(t, bindings[0].ExpiresAt)

	// Delete the binding
	require.NoError(t, store.RBAC().DeleteRoleBinding(ctx, 42, "deployer"))

	bindings, err = store.RBAC().GetUserRoleBindings(ctx, 42)
	require.NoError(t, err)
	assert.Empty(t, bindings)
}

func TestRBACRepo_RoleBinding_NoExpiry(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// FK: role must exist first
	require.NoError(t, store.RBAC().CreateRole(ctx, &entity.Role{
		Name: "viewer", DisplayName: "Viewer", CreatedAt: time.Now().UTC(),
	}))
	// FK: user must exist
	require.NoError(t, store.Users().Upsert(ctx, &entity.User{
		TelegramID: 100, Username: "noexpiry", Role: entity.RoleViewer, IsActive: true,
	}))

	b := &entity.UserRoleBinding{
		UserID:    100,
		RoleName:  "viewer",
		ExpiresAt: nil,
	}
	require.NoError(t, store.RBAC().CreateRoleBinding(ctx, b))

	bindings, err := store.RBAC().GetUserRoleBindings(ctx, 100)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	assert.Nil(t, bindings[0].ExpiresAt)
}

func TestRBACRepo_ListAllBindings(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// FK: role must exist first
	require.NoError(t, store.RBAC().CreateRole(ctx, &entity.Role{
		Name: "viewer", DisplayName: "Viewer", CreatedAt: time.Now().UTC(),
	}))

	for i, userID := range []int64{1, 2, 3} {
		// FK: user must exist
		require.NoError(t, store.Users().Upsert(ctx, &entity.User{
			TelegramID: userID,
			Username:   "binduser" + string(rune('0'+i)),
			Role:       entity.RoleViewer,
			IsActive:   true,
		}))
		require.NoError(t, store.RBAC().CreateRoleBinding(ctx, &entity.UserRoleBinding{
			UserID:   userID,
			RoleName: "viewer",
		}))
	}

	all, err := store.RBAC().ListAllBindings(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

// ─── approvalRepo CRUD ────────────────────────────────────────────────────────

func testApprovalRequest(id string) *entity.ApprovalRequest {
	return &entity.ApprovalRequest{
		ID:                id,
		RequesterID:       1,
		RequesterName:     "alice",
		Action:            "kubernetes.deployments.scale",
		Resource:          "deployment/api",
		Cluster:           "prod",
		Namespace:         "default",
		Details:           map[string]interface{}{"replicas": "5"},
		Status:            entity.ApprovalPending,
		RequiredApprovals: 2,
		ChatID:            100,
		MessageID:         200,
		CreatedAt:         time.Now().UTC().Truncate(time.Second),
		ExpiresAt:         time.Now().UTC().Add(15 * time.Minute).Truncate(time.Second),
	}
}

func TestApprovalRepo_CreateAndGetByID(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	req := testApprovalRequest("approval-001")
	require.NoError(t, store.Approval().Create(ctx, req))

	got, err := store.Approval().GetByID(ctx, "approval-001")
	require.NoError(t, err)
	assert.Equal(t, "approval-001", got.ID)
	assert.Equal(t, "alice", got.RequesterName)
	assert.Equal(t, entity.ApprovalPending, got.Status)
	assert.Equal(t, "prod", got.Cluster)
}

func TestApprovalRepo_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	_, err := store.Approval().GetByID(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestApprovalRepo_Update(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	req := testApprovalRequest("approval-002")
	require.NoError(t, store.Approval().Create(ctx, req))

	now := time.Now().UTC().Truncate(time.Second)
	req.Status = entity.ApprovalApproved
	req.Approvers = []entity.Approver{
		{UserID: 99, Decision: "approved"},
	}
	resolvedBy := int64(99)
	req.ResolvedAt = &now
	req.ResolvedBy = &resolvedBy
	require.NoError(t, store.Approval().Update(ctx, req))

	got, err := store.Approval().GetByID(ctx, "approval-002")
	require.NoError(t, err)
	assert.Equal(t, entity.ApprovalApproved, got.Status)
	assert.Len(t, got.Approvers, 1)
	assert.NotNil(t, got.ResolvedAt)
}

func TestApprovalRepo_ListPending(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// Insert one pending and one approved
	req1 := testApprovalRequest("req-1")
	req2 := testApprovalRequest("req-2")
	require.NoError(t, store.Approval().Create(ctx, req1))
	require.NoError(t, store.Approval().Create(ctx, req2))

	req2.Status = entity.ApprovalApproved
	require.NoError(t, store.Approval().Update(ctx, req2))

	pending, err := store.Approval().ListPending(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	assert.Equal(t, "req-1", pending[0].ID)
}

func TestApprovalRepo_ListByRequester(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		r := testApprovalRequest(string(rune('a' + i)))
		r.RequesterID = 42
		require.NoError(t, store.Approval().Create(ctx, r))
	}

	// Different requester
	other := testApprovalRequest("z")
	other.RequesterID = 99
	require.NoError(t, store.Approval().Create(ctx, other))

	list, err := store.Approval().ListByRequester(ctx, 42)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestApprovalRepo_ExpireOld(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx := context.Background()

	// Expired in the past
	expired := testApprovalRequest("exp-1")
	expired.ExpiresAt = time.Now().UTC().Add(-1 * time.Hour)
	require.NoError(t, store.Approval().Create(ctx, expired))

	// Still pending
	active := testApprovalRequest("exp-2")
	active.ExpiresAt = time.Now().UTC().Add(1 * time.Hour)
	require.NoError(t, store.Approval().Create(ctx, active))

	n, err := store.Approval().ExpireOld(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only 1 approval should be expired")

	got, err := store.Approval().GetByID(ctx, "exp-1")
	require.NoError(t, err)
	assert.Equal(t, entity.ApprovalExpired, got.Status)
}
