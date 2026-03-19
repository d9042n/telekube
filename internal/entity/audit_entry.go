package entity

import "time"

// AuditEntry records a single user action.
type AuditEntry struct {
	ID         string                 `json:"id"` // ULID
	UserID     int64                  `json:"user_id"`
	Username   string                 `json:"username"`
	Action     string                 `json:"action"`    // e.g. "pod.restart", "deployment.scale"
	Resource   string                 `json:"resource"`  // e.g. "pod/my-app-xyz"
	Cluster    string                 `json:"cluster"`
	Namespace  string                 `json:"namespace"`
	ChatID     int64                  `json:"chat_id"`
	ChatType   string                 `json:"chat_type"` // private, group
	Status     string                 `json:"status"`    // success, denied, error
	Details    map[string]interface{} `json:"details,omitempty"`
	Error      string                 `json:"error,omitempty"`
	OccurredAt time.Time              `json:"occurred_at"`
}

// Audit action status constants.
const (
	AuditStatusSuccess = "success"
	AuditStatusDenied  = "denied"
	AuditStatusError   = "error"
)
