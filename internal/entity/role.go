// Package entity defines domain models as pure Go structs.
package entity

import "time"

// PolicyRule defines a permission rule within a role.
type PolicyRule struct {
	Modules    []string `json:"modules"`    // ["kubernetes", "argocd"] or ["*"]
	Resources  []string `json:"resources"`  // ["pods", "deployments"] or ["*"]
	Actions    []string `json:"actions"`    // ["list", "get", "restart"] or ["*"]
	Clusters   []string `json:"clusters"`   // ["prod-1"] or ["*"]
	Namespaces []string `json:"namespaces"` // ["production"] or ["*"]
	Effect     string   `json:"effect"`     // "allow" or "deny"
}

// matchesWildcard checks if a value matches a list that may contain "*".
func matchesWildcard(list []string, value string) bool {
	for _, v := range list {
		if v == "*" || v == value {
			return true
		}
	}
	return false
}

// Matches returns true if the rule applies to the given permission request.
func (r PolicyRule) Matches(module, resource, action, cluster, namespace string) bool {
	return matchesWildcard(r.Modules, module) &&
		matchesWildcard(r.Resources, resource) &&
		matchesWildcard(r.Actions, action) &&
		matchesWildcard(r.Clusters, cluster) &&
		matchesWildcard(r.Namespaces, namespace)
}

// Role represents a named role with a set of policy rules.
type Role struct {
	Name        string       `json:"name"`
	DisplayName string       `json:"display_name"`
	Description string       `json:"description"`
	Rules       []PolicyRule `json:"rules"`
	IsBuiltin   bool         `json:"is_builtin"`
	CreatedAt   time.Time    `json:"created_at"`

	// Legacy flat permissions for backward compatibility (Phase 1–3 roles).
	Permissions []string `json:"permissions,omitempty"`
}

// UserRoleBinding binds a user to a role, optionally with expiry.
type UserRoleBinding struct {
	ID        string     `json:"id"` // ULID
	UserID    int64      `json:"user_id"`
	RoleName  string     `json:"role_name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// IsExpired returns true if the binding has a set expiry that has passed.
func (b UserRoleBinding) IsExpired() bool {
	if b.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*b.ExpiresAt)
}
