// Package entity defines domain models as pure Go structs.
package entity

import "time"

// User represents a Telegram user registered with the bot.
type User struct {
	TelegramID  int64     `json:"telegram_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"` // admin, operator, viewer
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// User roles.
const (
	RoleAdmin      = "admin"
	RoleOperator   = "operator"
	RoleViewer     = "viewer"
	RoleSuperAdmin = "super-admin"
	RoleOnCall     = "on-call"
)

// ValidRoles returns all valid role names.
func ValidRoles() []string {
	return []string{RoleSuperAdmin, RoleAdmin, RoleOperator, RoleViewer, RoleOnCall}
}

// IsValidRole checks if a role name is valid.
func IsValidRole(role string) bool {
	for _, r := range ValidRoles() {
		if r == role {
			return true
		}
	}
	return false
}
