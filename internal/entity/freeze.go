package entity

import "time"

// DeploymentFreeze represents an active deployment freeze that blocks sync/rollback/scale operations.
type DeploymentFreeze struct {
	ID        string     `json:"id"`         // ULID
	Scope     string     `json:"scope"`      // "all" or specific cluster name
	Reason    string     `json:"reason"`     // Optional human-readable reason
	CreatedBy int64      `json:"created_by"` // Telegram user ID
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	ThawedAt  *time.Time `json:"thawed_at,omitempty"` // Set when thawed early
	ThawedBy  *int64     `json:"thawed_by,omitempty"` // Telegram user ID
}

// IsActive returns true if the freeze is currently active (not expired, not thawed).
func (f *DeploymentFreeze) IsActive() bool {
	return f.ThawedAt == nil && time.Now().Before(f.ExpiresAt)
}

// RemainingDuration returns how much time is left in the freeze.
func (f *DeploymentFreeze) RemainingDuration() time.Duration {
	if !f.IsActive() {
		return 0
	}
	remaining := time.Until(f.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}
