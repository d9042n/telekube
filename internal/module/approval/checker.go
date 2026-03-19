// Package approval provides the approval checker interface used by other modules
// to gate dangerous operations behind approval workflows.
package approval

import (
	"context"

	"github.com/d9042n/telekube/internal/entity"
	"gopkg.in/telebot.v3"
)

// Checker is the interface that other modules (ArgoCD, K8s) use to check
// whether an action requires approval and to submit approval requests.
// This breaks the import cycle: modules depend on Checker, not on Manager directly.
type Checker interface {
	// CheckAndSubmit checks if the action needs approval. If yes, it creates an
	// approval request and sends the approval message to the chat. Returns true
	// if approval is required (action should NOT proceed immediately).
	// Returns false if no approval needed (action can proceed).
	CheckAndSubmit(ctx context.Context, c telebot.Context, req ApprovalInput) (bool, error)
}

// ApprovalInput contains the information needed to create an approval request.
type ApprovalInput struct {
	UserID      int64
	Username    string
	Action      string // e.g. "argocd.apps.sync"
	Resource    string // e.g. "payment-service"
	Cluster     string
	Namespace   string
	Details     map[string]interface{}
	CallbackData string // original callback data to replay after approval
}

// checker implements the Checker interface backed by the Manager.
type checker struct {
	manager *Manager
}

// NewChecker creates a Checker backed by the given Manager.
func NewChecker(manager *Manager) Checker {
	if manager == nil || !manager.cfg.Enabled {
		return &noopChecker{}
	}
	return &checker{manager: manager}
}

func (c *checker) CheckAndSubmit(ctx context.Context, tc telebot.Context, input ApprovalInput) (bool, error) {
	rule := c.manager.RequiresApproval(input.Action, input.Cluster)
	if rule == nil {
		return false, nil // No approval needed
	}

	// Build approval request
	req := &entity.ApprovalRequest{
		RequesterID:   input.UserID,
		RequesterName: input.Username,
		Action:        input.Action,
		Resource:      input.Resource,
		Cluster:       input.Cluster,
		Namespace:     input.Namespace,
		Details:       input.Details,
	}

	if err := c.manager.Submit(ctx, req); err != nil {
		return false, err
	}

	// Send approval message with buttons
	text := BuildApprovalMessage(req)
	kbd := ApprovalKeyboard(req)
	msg, err := tc.Bot().Send(tc.Chat(), text, kbd)
	if err == nil && msg != nil {
		// Store message ID so we can edit it when approved/rejected
		req.MessageID = msg.ID
		req.ChatID = tc.Chat().ID
		_ = c.manager.storage.Update(ctx, req)
	}

	// Notify the requester
	_ = tc.Respond(&telebot.CallbackResponse{
		Text: "📋 Approval request submitted. Waiting for approval...",
	})

	return true, nil
}

// noopChecker always returns false (no approval needed).
type noopChecker struct{}

func (n *noopChecker) CheckAndSubmit(_ context.Context, _ telebot.Context, _ ApprovalInput) (bool, error) {
	return false, nil
}
