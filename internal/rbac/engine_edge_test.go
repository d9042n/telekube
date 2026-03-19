package rbac

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── errRBACRepo — a repo that always returns errors ─────────────────────────

type errRBACRepo struct {
	mockRBACRepo
}

func (e *errRBACRepo) GetUserRole(_ context.Context, _ int64) (string, error) {
	return "", errors.New("db is down")
}

func (e *errRBACRepo) GetUserRoleBindings(_ context.Context, _ int64) ([]entity.UserRoleBinding, error) {
	return nil, errors.New("db is down")
}

// ─── Expired binding edge cases ───────────────────────────────────────────────

func TestEngine_Authorize_ExpiredBinding_Ignored(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	eng := NewEngine(entity.RoleViewer, repo, nil)

	// Create an admin-like custom role.
	require.NoError(t, eng.CreateRole(context.Background(), &entity.Role{
		Name: "custom-admin",
		Rules: []entity.PolicyRule{
			{
				Modules:    []string{"*"},
				Resources:  []string{"*"},
				Actions:    []string{"*"},
				Clusters:   []string{"*"},
				Namespaces: []string{"*"},
				Effect:     "allow",
			},
		},
	}))

	// Bind user 100 to the custom-admin role, but with an already-expired expiry.
	past := time.Now().Add(-time.Hour)
	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID:    100,
		RoleName:  "custom-admin",
		ExpiresAt: &past,
	}))

	// Storage returns the binding, but it's expired — should fall through to legacy viewer perms.
	ok, err := eng.Authorize(context.Background(), 100, PermissionRequest{
		Module:    "kubernetes",
		Resource:  "pods",
		Action:    "delete", // viewer cannot delete
		Cluster:   "prod-1",
		Namespace: "default",
	})
	require.NoError(t, err)
	assert.False(t, ok, "expired binding must not grant permission")
}

// ─── Deny overrides allow with multiple roles ─────────────────────────────────

func TestEngine_Authorize_MultiRole_DenyWins(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	eng := NewEngine(entity.RoleViewer, repo, nil)

	// Role 1: allow everything.
	require.NoError(t, eng.CreateRole(context.Background(), &entity.Role{
		Name: "allow-all",
		Rules: []entity.PolicyRule{{
			Modules:    []string{"*"},
			Resources:  []string{"*"},
			Actions:    []string{"*"},
			Clusters:   []string{"*"},
			Namespaces: []string{"*"},
			Effect:     "allow",
		}},
	}))
	// Role 2: deny kubernetes.nodes.drain on prod.
	require.NoError(t, eng.CreateRole(context.Background(), &entity.Role{
		Name: "deny-drain-prod",
		Rules: []entity.PolicyRule{{
			Modules:    []string{"kubernetes"},
			Resources:  []string{"nodes"},
			Actions:    []string{"drain"},
			Clusters:   []string{"prod-1"},
			Namespaces: []string{"*"},
			Effect:     "deny",
		}},
	}))

	// Bind user 200 to both roles.
	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 200, RoleName: "allow-all",
	}))
	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 200, RoleName: "deny-drain-prod",
	}))

	// Deny should win.
	ok, err := eng.Authorize(context.Background(), 200, PermissionRequest{
		Module:    "kubernetes",
		Resource:  "nodes",
		Action:    "drain",
		Cluster:   "prod-1",
		Namespace: "default",
	})
	require.NoError(t, err)
	assert.False(t, ok, "deny must override allow when user has multiple roles")

	// But on staging cluster, drain is allowed.
	ok, err = eng.Authorize(context.Background(), 200, PermissionRequest{
		Module:    "kubernetes",
		Resource:  "nodes",
		Action:    "drain",
		Cluster:   "staging",
		Namespace: "default",
	})
	require.NoError(t, err)
	assert.True(t, ok, "drain on staging should be allowed (deny only covers prod-1)")
}

// ─── Empty rules ──────────────────────────────────────────────────────────────

func TestEngine_Authorize_EmptyRules_FallsBackToLegacy(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	eng := NewEngine(entity.RoleViewer, repo, nil)

	// Custom role with no rules.
	require.NoError(t, eng.CreateRole(context.Background(), &entity.Role{
		Name:  "empty-role",
		Rules: []entity.PolicyRule{},
	}))
	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 300, RoleName: "empty-role",
	}))

	// No dynamic match → falls back to legacy flat role (viewer by default).
	ok, err := eng.Authorize(context.Background(), 300, PermissionRequest{
		Module:    "kubernetes",
		Resource:  "pods",
		Action:    "list", // viewer can list
		Cluster:   "prod-1",
		Namespace: "default",
	})
	require.NoError(t, err)
	assert.True(t, ok, "viewer can list when no dynamic rules matched")
}

// ─── Super-admin bypasses all permission checks ───────────────────────────────

func TestEngine_Authorize_SuperAdmin_BypassAll(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	superAdminID := int64(999)
	eng := NewEngine(entity.RoleViewer, repo, []int64{superAdminID})

	// Super-admin should be able to do anything, without any role binding.
	actions := []PermissionRequest{
		{Module: "kubernetes", Resource: "pods", Action: "delete", Cluster: "prod-1", Namespace: "kube-system"},
		{Module: "admin", Resource: "rbac", Action: "manage", Cluster: "prod-1", Namespace: "default"},
		{Module: "argocd", Resource: "apps", Action: "rollback", Cluster: "prod-1", Namespace: "default"},
		{Module: "helm", Resource: "releases", Action: "rollback", Cluster: "prod-1", Namespace: "default"},
	}
	for _, req := range actions {
		ok, err := eng.Authorize(context.Background(), superAdminID, req)
		require.NoError(t, err)
		assert.True(t, ok, "super-admin must be allowed for %s", req)
	}
}

func TestEngine_HasPermission_SuperAdmin_BypassAll(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	// User 999 is not assigned any role in storage.
	eng := NewEngine(entity.RoleViewer, repo, []int64{999})

	ok, err := eng.HasPermission(context.Background(), 999, PermKubernetesPodsDelete)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = eng.HasPermission(context.Background(), 999, PermAdminRBACManage)
	require.NoError(t, err)
	assert.True(t, ok)
}

// ─── Binding for non-existent role falls through to legacy ───────────────────

func TestEngine_Authorize_UnknownRole_FallsBackToLegacy(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	eng := NewEngine(entity.RoleViewer, repo, nil)

	// Bind user 400 to a role that does not exist in storage.
	repo.bindings = append(repo.bindings, entity.UserRoleBinding{
		UserID:   400,
		RoleName: "non-existent-role",
	})

	// GetRole will return ErrNotFound for "non-existent-role" → binding is skipped.
	// No dynamic match → legacy viewer rules apply.
	ok, err := eng.Authorize(context.Background(), 400, PermissionRequest{
		Module:    "kubernetes",
		Resource:  "pods",
		Action:    "list",
		Cluster:   "prod-1",
		Namespace: "default",
	})
	require.NoError(t, err)
	assert.True(t, ok, "should fall back to legacy viewer (can list)")
}

// ─── SetRole with invalid role ────────────────────────────────────────────────

func TestEngine_SetRole_InvalidRole_ReturnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role string
	}{
		{""},
		{"superuser"},
		{"ADMIN"},
		{"root"},
	}

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), nil)
	for _, tt := range tests {
		err := eng.SetRole(context.Background(), 100, tt.role)
		assert.Error(t, err, "role %q should be rejected", tt.role)
	}
}

// ─── GetRole storage error propagated ────────────────────────────────────────

func TestEngine_GetRole_StorageError_Propagated(t *testing.T) {
	t.Parallel()

	errRepo := &errRBACRepo{}
	eng := NewEngine(entity.RoleViewer, errRepo, nil)

	_, err := eng.GetRole(context.Background(), 100)
	assert.Error(t, err, "storage error must be propagated from GetRole")
}

// ─── Authorize with storage error, falls through to legacy ───────────────────

func TestEngine_Authorize_BindingStorageError_FallsBackToLegacy(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	repo.roles[100] = entity.RoleAdmin

	eng := NewEngine(entity.RoleViewer, repo, nil)

	// Normal user with admin role in legacy storage: binding retrieval may fail
	// but HasPermission still works from legacy.
	ok, err := eng.HasPermission(context.Background(), 100, PermAdminRBACManage)
	require.NoError(t, err)
	assert.True(t, ok)
}

// ─── Multiple expired + valid bindings ───────────────────────────────────────

func TestEngine_Authorize_MixedExpiredAndValid_OnlyValidCounts(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	eng := NewEngine(entity.RoleViewer, repo, nil)

	// A powerful role that allows everything.
	require.NoError(t, eng.CreateRole(context.Background(), &entity.Role{
		Name: "powerful",
		Rules: []entity.PolicyRule{{
			Modules:    []string{"*"},
			Resources:  []string{"*"},
			Actions:    []string{"*"},
			Clusters:   []string{"*"},
			Namespaces: []string{"*"},
			Effect:     "allow",
		}},
	}))
	// A restrictive role that only allows listing.
	require.NoError(t, eng.CreateRole(context.Background(), &entity.Role{
		Name: "list-only",
		Rules: []entity.PolicyRule{{
			Modules:    []string{"kubernetes"},
			Resources:  []string{"pods"},
			Actions:    []string{"list"},
			Clusters:   []string{"*"},
			Namespaces: []string{"*"},
			Effect:     "allow",
		}},
	}))

	past := time.Now().Add(-time.Hour)
	// Bind user to powerful (EXPIRED) + list-only (VALID).
	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 500, RoleName: "powerful", ExpiresAt: &past,
	}))
	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 500, RoleName: "list-only",
	}))

	// Delete is not allowed — powerful is expired; list-only doesn't grant it.
	ok, err := eng.Authorize(context.Background(), 500, PermissionRequest{
		Module:    "kubernetes",
		Resource:  "pods",
		Action:    "delete",
		Cluster:   "prod-1",
		Namespace: "default",
	})
	require.NoError(t, err)
	assert.False(t, ok, "expired powerful binding must not grant delete")

	// List is allowed via the valid list-only binding.
	ok, err = eng.Authorize(context.Background(), 500, PermissionRequest{
		Module:    "kubernetes",
		Resource:  "pods",
		Action:    "list",
		Cluster:   "prod-1",
		Namespace: "default",
	})
	require.NoError(t, err)
	assert.True(t, ok, "valid list-only binding must grant list")
}

// ─── AssignRole + ListUserBindings ───────────────────────────────────────────

func TestEngine_AssignRole_And_ListBindings(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	eng := NewEngine(entity.RoleViewer, repo, nil)

	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 100, RoleName: entity.RoleAdmin,
	}))
	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 100, RoleName: entity.RoleOperator,
	}))

	bindings, err := eng.ListUserBindings(context.Background(), 100)
	require.NoError(t, err)
	assert.Len(t, bindings, 2)
}

func TestEngine_AssignRole_MissingUserID_ReturnsError(t *testing.T) {
	t.Parallel()

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), nil)
	err := eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 0, RoleName: entity.RoleAdmin,
	})
	assert.Error(t, err)
}

func TestEngine_AssignRole_MissingRoleName_ReturnsError(t *testing.T) {
	t.Parallel()

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), nil)
	err := eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 100, RoleName: "",
	})
	assert.Error(t, err)
}

// ─── storage.ErrNotFound → returns defaultRole ───────────────────────────────

func TestEngine_GetRole_NotFound_ReturnsDefault(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	// Unknown user — not in storage.
	eng := NewEngine(entity.RoleViewer, repo, nil)

	role, err := eng.GetRole(context.Background(), 12345)
	require.NoError(t, err)
	assert.Equal(t, entity.RoleViewer, role, "unknown user must default to viewer")
}

func TestEngine_GetRole_AdminDefault(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	// Use admin as the defaultRole.
	eng := NewEngine(entity.RoleAdmin, repo, nil)

	role, err := eng.GetRole(context.Background(), 99999)
	require.NoError(t, err)
	assert.Equal(t, entity.RoleAdmin, role, "default role should be admin when configured so")
}

// ─── RevokeRole ───────────────────────────────────────────────────────────────

func TestEngine_RevokeRole(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	eng := NewEngine(entity.RoleViewer, repo, nil)

	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 100, RoleName: entity.RoleAdmin,
	}))

	bindings, _ := eng.ListUserBindings(context.Background(), 100)
	assert.Len(t, bindings, 1)

	require.NoError(t, eng.RevokeRole(context.Background(), 100, entity.RoleAdmin))

	bindings, _ = eng.ListUserBindings(context.Background(), 100)
	assert.Empty(t, bindings)
}

// ─── IsSuperAdmin with empty admin list ──────────────────────────────────────

func TestEngine_IsSuperAdmin_EmptyList(t *testing.T) {
	t.Parallel()

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), nil)
	assert.False(t, eng.IsSuperAdmin(1))
	assert.False(t, eng.IsSuperAdmin(0))
}

// ─── Roles() returns built-in roles ──────────────────────────────────────────

func TestEngine_Roles_ReturnsBuiltins(t *testing.T) {
	t.Parallel()

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), nil)
	roles := eng.Roles()

	names := make(map[string]bool)
	for _, r := range roles {
		names[r.Name] = true
	}
	assert.True(t, names[entity.RoleViewer], "viewer must be in built-in roles")
	assert.True(t, names[entity.RoleOperator], "operator must be in built-in roles")
	assert.True(t, names[entity.RoleAdmin], "admin must be in built-in roles")
}

// ─── ListAllBindings ──────────────────────────────────────────────────────────

func TestEngine_ListAllBindings(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	eng := NewEngine(entity.RoleViewer, repo, nil)

	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 10, RoleName: entity.RoleAdmin,
	}))
	require.NoError(t, eng.AssignRole(context.Background(), &entity.UserRoleBinding{
		UserID: 20, RoleName: entity.RoleViewer,
	}))

	all, err := eng.ListAllBindings(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

// ─── ListRoles (custom roles) ─────────────────────────────────────────────────

func TestEngine_ListRoles_ReturnsPersisted(t *testing.T) {
	t.Parallel()

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), nil)

	require.NoError(t, eng.CreateRole(context.Background(), &entity.Role{
		Name: "custom-role-a",
	}))
	require.NoError(t, eng.CreateRole(context.Background(), &entity.Role{
		Name: "custom-role-b",
	}))

	roles, err := eng.ListRoles(context.Background())
	require.NoError(t, err)
	assert.Len(t, roles, 2)
}

// ─── CreateRole with empty name ───────────────────────────────────────────────

func TestEngine_CreateRole_EmptyName_ReturnsError(t *testing.T) {
	t.Parallel()

	eng := NewEngine(entity.RoleViewer, newMockRBACRepo(), nil)
	err := eng.CreateRole(context.Background(), &entity.Role{Name: ""})
	assert.Error(t, err, "creating a role with empty name must fail")
}

// ─── PermissionRequest.String ─────────────────────────────────────────────────

func TestPermissionRequest_String(t *testing.T) {
	t.Parallel()

	req := PermissionRequest{
		Module:    "kubernetes",
		Resource:  "pods",
		Action:    "restart",
		Cluster:   "prod-1",
		Namespace: "production",
	}
	s := req.String()
	assert.Contains(t, s, "kubernetes")
	assert.Contains(t, s, "pods")
	assert.Contains(t, s, "restart")
	assert.Contains(t, s, "prod-1")
	assert.Contains(t, s, "production")
}

// ─── HasPermission for a permission not in any built-in role ─────────────────

func TestEngine_HasPermission_UnknownPermission_ReturnsFalse(t *testing.T) {
	t.Parallel()

	repo := newMockRBACRepo()
	repo.roles[100] = entity.RoleAdmin
	eng := NewEngine(entity.RoleViewer, repo, nil)

	ok, err := eng.HasPermission(context.Background(), 100, "totally.unknown.permission")
	require.NoError(t, err)
	assert.False(t, ok, "unknown permission must return false even for admin")
}

// ─── HasPermission when storage returns error for GetRole ─────────────────────

func TestEngine_HasPermission_GetRoleError_ReturnsError(t *testing.T) {
	t.Parallel()

	errRepo := &errRBACRepo{}
	eng := NewEngine(entity.RoleViewer, errRepo, nil)

	_, err := eng.HasPermission(context.Background(), 100, PermKubernetesPodsList)
	assert.Error(t, err, "HasPermission must propagate storage errors from GetRole")
}

