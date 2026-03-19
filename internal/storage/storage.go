// Package storage defines repository interfaces for data access.
package storage

import (
	"context"
	"time"

	"github.com/d9042n/telekube/internal/entity"
)

// Storage is the top-level data access interface.
type Storage interface {
	Users() UserRepository
	Audit() AuditRepository
	RBAC() RBACRepository
	Freeze() FreezeRepository
	Approval() ApprovalRepository
	NotificationPrefs() NotificationPrefRepository
	Close() error
	Ping(ctx context.Context) error
}

// UserRepository manages user persistence.
type UserRepository interface {
	GetByTelegramID(ctx context.Context, telegramID int64) (*entity.User, error)
	Upsert(ctx context.Context, user *entity.User) error
	List(ctx context.Context) ([]entity.User, error)
}

// AuditRepository manages audit log persistence.
type AuditRepository interface {
	Create(ctx context.Context, entry *entity.AuditEntry) error
	List(ctx context.Context, filter AuditFilter) ([]entity.AuditEntry, int, error)
}

// AuditFilter defines query filters for audit log.
type AuditFilter struct {
	UserID    *int64
	Action    *string
	Cluster   *string
	Namespace *string
	Status    *string
	From      *time.Time
	To        *time.Time
	Page      int
	PageSize  int
}

// FreezeRepository manages deployment freeze state.
type FreezeRepository interface {
	// Create inserts a new deployment freeze.
	Create(ctx context.Context, freeze *entity.DeploymentFreeze) error
	// GetActive returns the currently active freeze (not expired, not thawed), or nil.
	GetActive(ctx context.Context) (*entity.DeploymentFreeze, error)
	// GetActiveForCluster returns the active freeze that covers the given cluster (or "all" scope).
	GetActiveForCluster(ctx context.Context, clusterName string) (*entity.DeploymentFreeze, error)
	// Thaw marks a freeze as thawed early.
	Thaw(ctx context.Context, id string, thawedBy int64) error
	// List returns historical freeze entries, newest first.
	List(ctx context.Context, limit int) ([]entity.DeploymentFreeze, error)
}

// RBACRepository manages role assignments and custom role definitions.
type RBACRepository interface {
	// Legacy flat-role helpers (Phase 1–3 compat).
	GetUserRole(ctx context.Context, telegramID int64) (string, error)
	SetUserRole(ctx context.Context, telegramID int64, role string) error

	// Phase 4 — custom roles.
	CreateRole(ctx context.Context, role *entity.Role) error
	GetRole(ctx context.Context, name string) (*entity.Role, error)
	ListRoles(ctx context.Context) ([]entity.Role, error)
	DeleteRole(ctx context.Context, name string) error

	// Phase 4 — role bindings.
	CreateRoleBinding(ctx context.Context, binding *entity.UserRoleBinding) error
	GetUserRoleBindings(ctx context.Context, userID int64) ([]entity.UserRoleBinding, error)
	DeleteRoleBinding(ctx context.Context, userID int64, roleName string) error
	ListAllBindings(ctx context.Context) ([]entity.UserRoleBinding, error)
}

// ApprovalRepository manages approval request persistence.
type ApprovalRepository interface {
	Create(ctx context.Context, req *entity.ApprovalRequest) error
	GetByID(ctx context.Context, id string) (*entity.ApprovalRequest, error)
	Update(ctx context.Context, req *entity.ApprovalRequest) error
	ListPending(ctx context.Context) ([]entity.ApprovalRequest, error)
	ListByRequester(ctx context.Context, userID int64) ([]entity.ApprovalRequest, error)
	ExpireOld(ctx context.Context) (int, error) // returns number of expired requests
}

// NotificationPrefRepository manages per-user notification preferences.
type NotificationPrefRepository interface {
	Get(ctx context.Context, userID int64) (*entity.NotificationPreference, error)
	Upsert(ctx context.Context, pref *entity.NotificationPreference) error
	Delete(ctx context.Context, userID int64) error
}
