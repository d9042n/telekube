package entity

import "time"

// ApprovalStatus represents the state of an approval request.
type ApprovalStatus string

const (
	ApprovalPending   ApprovalStatus = "pending"
	ApprovalApproved  ApprovalStatus = "approved"
	ApprovalRejected  ApprovalStatus = "rejected"
	ApprovalExpired   ApprovalStatus = "expired"
	ApprovalCancelled ApprovalStatus = "cancelled"
)

// Approver tracks an individual approver's decision.
type Approver struct {
	UserID    int64      `json:"user_id"`
	Decision  string     `json:"decision"` // "pending", "approved", "rejected"
	Comment   string     `json:"comment,omitempty"`
	DecidedAt *time.Time `json:"decided_at,omitempty"`
}

// ApprovalRequest represents a pending, approved, or resolved approval workflow.
type ApprovalRequest struct {
	ID                string                 `json:"id"` // ULID
	RequesterID       int64                  `json:"requester_id"`
	RequesterName     string                 `json:"requester_name"`
	Action            string                 `json:"action"`    // e.g. "argocd.apps.sync"
	Resource          string                 `json:"resource"`  // e.g. "payment-service"
	Cluster           string                 `json:"cluster"`
	Namespace         string                 `json:"namespace"`
	Details           map[string]interface{} `json:"details"` // action-specific payload
	Status            ApprovalStatus         `json:"status"`
	Approvers         []Approver             `json:"approvers"`
	RequiredApprovals int                    `json:"required_approvals"`
	CreatedAt         time.Time              `json:"created_at"`
	ExpiresAt         time.Time              `json:"expires_at"`
	ResolvedAt        *time.Time             `json:"resolved_at,omitempty"`
	ResolvedBy        *int64                 `json:"resolved_by,omitempty"`

	// ChatID is the Telegram chat where the approval was requested.
	ChatID int64 `json:"chat_id"`
	// MessageID is the Telegram message ID of the approval request (for editing).
	MessageID int `json:"message_id"`
}

// IsPending returns true if the request is still awaiting decision.
func (r *ApprovalRequest) IsPending() bool {
	return r.Status == ApprovalPending
}

// ApprovalCount returns the number of "approved" decisions.
func (r *ApprovalRequest) ApprovalCount() int {
	n := 0
	for _, a := range r.Approvers {
		if a.Decision == "approved" {
			n++
		}
	}
	return n
}

// HasRejection returns true if at least one approver rejected.
func (r *ApprovalRequest) HasRejection() bool {
	for _, a := range r.Approvers {
		if a.Decision == "rejected" {
			return true
		}
	}
	return false
}
