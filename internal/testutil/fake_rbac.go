package testutil

import (
	"context"
	"fmt"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/rbac"
)

// FakeRBAC is a simple in-memory RBAC engine for testing.
// Permission outcomes are controlled by the AllowAll flag or per-permission map.
type FakeRBAC struct {
	// AllowAll grants every permission regardless of permission string.
	AllowAll bool
	// DenyAll denies every permission. Overrides AllowAll.
	DenyAll bool
	// Allowed is a set of permission strings that return true.
	// Only consulted when AllowAll=false and DenyAll=false.
	Allowed map[string]bool
	// SuperAdminIDs is the set of user IDs that bypass all checks.
	SuperAdminIDs map[int64]bool
	// Role is the role returned by GetRole for any user.
	Role string
	// GetRoleErr, if set, is returned by GetRole.
	GetRoleErr error
}

// NewAllowAllRBAC creates a permissive RBAC engine that allows everything.
func NewAllowAllRBAC() *FakeRBAC {
	return &FakeRBAC{AllowAll: true, Role: entity.RoleAdmin}
}

// NewDenyAllRBAC creates a restrictive RBAC engine that denies everything.
func NewDenyAllRBAC() *FakeRBAC {
	return &FakeRBAC{DenyAll: true, Role: entity.RoleViewer}
}

// NewViewerRBAC creates a viewer-level RBAC engine.
// Pass the permissions a viewer is allowed to use.
func NewViewerRBAC(allowedPerms ...string) *FakeRBAC {
	allowed := make(map[string]bool, len(allowedPerms))
	for _, p := range allowedPerms {
		allowed[p] = true
	}
	return &FakeRBAC{Allowed: allowed, Role: entity.RoleViewer}
}

func (f *FakeRBAC) HasPermission(_ context.Context, userID int64, perm string) (bool, error) {
	if f.SuperAdminIDs[userID] {
		return true, nil
	}
	if f.DenyAll {
		return false, nil
	}
	if f.AllowAll {
		return true, nil
	}
	return f.Allowed[perm], nil
}

func (f *FakeRBAC) Authorize(_ context.Context, userID int64, _ rbac.PermissionRequest) (bool, error) {
	if f.SuperAdminIDs[userID] {
		return true, nil
	}
	if f.DenyAll {
		return false, nil
	}
	return f.AllowAll, nil
}

func (f *FakeRBAC) GetRole(_ context.Context, _ int64) (string, error) {
	if f.GetRoleErr != nil {
		return "", f.GetRoleErr
	}
	if f.Role == "" {
		return entity.RoleViewer, nil
	}
	return f.Role, nil
}

func (f *FakeRBAC) SetRole(_ context.Context, _ int64, role string) error {
	if !entity.IsValidRole(role) {
		return fmt.Errorf("invalid role: %s", role)
	}
	f.Role = role
	return nil
}

func (f *FakeRBAC) RolePermissions(_ string) []string { return nil }

func (f *FakeRBAC) IsSuperAdmin(userID int64) bool { return f.SuperAdminIDs[userID] }

func (f *FakeRBAC) Roles() []entity.Role { return nil }

func (f *FakeRBAC) CreateRole(_ context.Context, _ *entity.Role) error { return nil }

func (f *FakeRBAC) ListRoles(_ context.Context) ([]entity.Role, error) { return nil, nil }

func (f *FakeRBAC) AssignRole(_ context.Context, _ *entity.UserRoleBinding) error { return nil }

func (f *FakeRBAC) RevokeRole(_ context.Context, _ int64, _ string) error { return nil }

func (f *FakeRBAC) ListUserBindings(_ context.Context, _ int64) ([]entity.UserRoleBinding, error) {
	return nil, nil
}

func (f *FakeRBAC) ListAllBindings(_ context.Context) ([]entity.UserRoleBinding, error) {
	return nil, nil
}
