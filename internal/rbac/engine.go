// Package rbac provides role-based access control.
package rbac

import (
	"context"
	"fmt"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/oklog/ulid/v2"
)

// PermissionRequest encodes what the caller wants to do.
type PermissionRequest struct {
	Module    string // "kubernetes"
	Resource  string // "pods"
	Action    string // "restart"
	Cluster   string // "prod-1"
	Namespace string // "production"
}

// String returns a dotted representation for logging.
func (p PermissionRequest) String() string {
	return fmt.Sprintf("%s.%s.%s@%s/%s", p.Module, p.Resource, p.Action, p.Cluster, p.Namespace)
}

// Engine provides permission checking and role management.
type Engine interface {
	// HasPermission evaluates whether userID can perform a flat permission string (Phase 1–3 compat).
	HasPermission(ctx context.Context, userID int64, permission string) (bool, error)
	// Authorize evaluates a structured PermissionRequest (Phase 4 policy rules).
	Authorize(ctx context.Context, userID int64, req PermissionRequest) (bool, error)
	// GetRole returns the legacy flat role for a user.
	GetRole(ctx context.Context, userID int64) (string, error)
	// SetRole sets the legacy flat role for a user.
	SetRole(ctx context.Context, userID int64, role string) error
	// RolePermissions returns the flat permissions for a legacy role.
	RolePermissions(role string) []string
	// IsSuperAdmin returns true if the user is in the super-admin list.
	IsSuperAdmin(userID int64) bool
	// Roles returns all built-in role definitions.
	Roles() []entity.Role
	// CreateRole persists a custom role.
	CreateRole(ctx context.Context, role *entity.Role) error
	// ListRoles returns all persisted custom roles.
	ListRoles(ctx context.Context) ([]entity.Role, error)
	// AssignRole binds a role to a user (with optional expiry).
	AssignRole(ctx context.Context, binding *entity.UserRoleBinding) error
	// RevokeRole removes a binding.
	RevokeRole(ctx context.Context, userID int64, roleName string) error
	// ListUserBindings returns all role bindings for a user.
	ListUserBindings(ctx context.Context, userID int64) ([]entity.UserRoleBinding, error)
	// ListAllBindings returns all bindings in the system.
	ListAllBindings(ctx context.Context) ([]entity.UserRoleBinding, error)
}

type engine struct {
	storage     storage.RBACRepository
	superAdmins map[int64]bool
	defaultRole string
	// built-in legacy roles keyed by name
	builtinRoles    map[string]*entity.Role
	builtinPerms    map[string]map[string]bool // role -> permission -> allowed
}

// NewEngine creates a new RBAC engine.
func NewEngine(defaultRole string, store storage.RBACRepository, adminIDs []int64) Engine {
	superAdmins := make(map[int64]bool, len(adminIDs))
	for _, id := range adminIDs {
		superAdmins[id] = true
	}

	if defaultRole == "" {
		defaultRole = entity.RoleViewer
	}

	e := &engine{
		storage:      store,
		superAdmins:  superAdmins,
		defaultRole:  defaultRole,
		builtinRoles: make(map[string]*entity.Role),
		builtinPerms: make(map[string]map[string]bool),
	}

	e.initDefaultRoles()
	return e
}

func (e *engine) initDefaultRoles() {
	for _, role := range defaultRoles() {
		r := role
		e.builtinRoles[r.Name] = &r
		e.builtinPerms[r.Name] = make(map[string]bool)
		for _, perm := range r.Permissions {
			e.builtinPerms[r.Name][perm] = true
		}
	}
}

// HasPermission checks a flat permission string (Phase 1–3 backward compat).
func (e *engine) HasPermission(ctx context.Context, userID int64, permission string) (bool, error) {
	if e.IsSuperAdmin(userID) {
		return true, nil
	}

	role, err := e.GetRole(ctx, userID)
	if err != nil {
		return false, err
	}

	perms, ok := e.builtinPerms[role]
	if !ok {
		return false, nil
	}

	return perms[permission], nil
}

// Authorize evaluates a structured PermissionRequest using Phase 4 policy rules.
// Deny overrides allow: any matching deny rule causes rejection.
// Falls back to legacy flat role check if no dynamic bindings apply.
func (e *engine) Authorize(ctx context.Context, userID int64, req PermissionRequest) (bool, error) {
	if e.IsSuperAdmin(userID) {
		return true, nil
	}

	// 1. Evaluate Phase 4 dynamic role bindings.
	bindings, err := e.storage.GetUserRoleBindings(ctx, userID)
	if err == nil && len(bindings) > 0 {
		var allowed bool
		var hasAnyMatch bool

		for _, binding := range bindings {
			if binding.IsExpired() {
				continue
			}
			role, err := e.storage.GetRole(ctx, binding.RoleName)
			if err != nil {
				continue
			}
			for _, rule := range role.Rules {
				if !rule.Matches(req.Module, req.Resource, req.Action, req.Cluster, req.Namespace) {
					continue
				}
				hasAnyMatch = true
				if rule.Effect == "deny" {
					// Deny overrides everything.
					return false, nil
				}
				allowed = true
			}
		}

		if hasAnyMatch {
			return allowed, nil
		}
	}

	// 2. Fall back to legacy flat permission string.
	flatPerm := fmt.Sprintf("%s.%s.%s", req.Module, req.Resource, req.Action)
	return e.HasPermission(ctx, userID, flatPerm)
}

func (e *engine) GetRole(ctx context.Context, userID int64) (string, error) {
	if e.IsSuperAdmin(userID) {
		return entity.RoleAdmin, nil
	}

	role, err := e.storage.GetUserRole(ctx, userID)
	if err != nil {
		if err == storage.ErrNotFound {
			return e.defaultRole, nil
		}
		return "", fmt.Errorf("getting user role: %w", err)
	}
	return role, nil
}

func (e *engine) SetRole(ctx context.Context, userID int64, role string) error {
	if !entity.IsValidRole(role) {
		return fmt.Errorf("invalid role: %s", role)
	}
	return e.storage.SetUserRole(ctx, userID, role)
}

func (e *engine) RolePermissions(role string) []string {
	r, ok := e.builtinRoles[role]
	if !ok {
		return nil
	}
	return r.Permissions
}

func (e *engine) IsSuperAdmin(userID int64) bool {
	return e.superAdmins[userID]
}

func (e *engine) Roles() []entity.Role {
	result := make([]entity.Role, 0, len(e.builtinRoles))
	for _, r := range e.builtinRoles {
		result = append(result, *r)
	}
	return result
}

func (e *engine) CreateRole(ctx context.Context, role *entity.Role) error {
	if role.Name == "" {
		return fmt.Errorf("role name is required")
	}
	return e.storage.CreateRole(ctx, role)
}

func (e *engine) ListRoles(ctx context.Context) ([]entity.Role, error) {
	return e.storage.ListRoles(ctx)
}

func (e *engine) AssignRole(ctx context.Context, binding *entity.UserRoleBinding) error {
	if binding.UserID == 0 {
		return fmt.Errorf("user_id is required")
	}
	if binding.RoleName == "" {
		return fmt.Errorf("role_name is required")
	}
	if binding.ID == "" {
		binding.ID = ulid.Make().String()
	}
	return e.storage.CreateRoleBinding(ctx, binding)
}

func (e *engine) RevokeRole(ctx context.Context, userID int64, roleName string) error {
	return e.storage.DeleteRoleBinding(ctx, userID, roleName)
}

func (e *engine) ListUserBindings(ctx context.Context, userID int64) ([]entity.UserRoleBinding, error) {
	return e.storage.GetUserRoleBindings(ctx, userID)
}

func (e *engine) ListAllBindings(ctx context.Context) ([]entity.UserRoleBinding, error) {
	return e.storage.ListAllBindings(ctx)
}
