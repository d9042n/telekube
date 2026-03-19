// Package approval implements the approval workflow module for Telekube.
package approval

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
)

// Rule describes when approval is required for an action.
type Rule struct {
	Action            string   // e.g. "argocd.apps.sync"
	Clusters          []string // ["prod-1"] or ["*"]
	RequiredApprovals int
	ApproverRoles     []string // ["admin", "super-admin"]
}

// Config holds the approval workflow configuration.
type Config struct {
	Enabled       bool
	DefaultExpiry time.Duration
	Rules         []Rule
}

// RoleChecker resolves whether a user has a given role.
type RoleChecker interface {
	GetRole(ctx context.Context, userID int64) (string, error)
}

// Notifier sends Telegram messages.
type Notifier interface {
	SendApprovalRequest(chatID int64, req *entity.ApprovalRequest) (int, error)
	SendApprovalResult(chatID int64, req *entity.ApprovalRequest, comment string) error
	NotifyRequester(userID int64, req *entity.ApprovalRequest, approved bool, comment string) error
}

// Manager orchestrates the approval workflow.
type Manager struct {
	cfg      Config
	storage  storage.ApprovalRepository
	roles    RoleChecker
	notifier Notifier
	logger   *zap.Logger
	mu       sync.Mutex
}

// New creates a new approval Manager.
func New(cfg Config, store storage.ApprovalRepository, roles RoleChecker, notifier Notifier, logger *zap.Logger) *Manager {
	if cfg.DefaultExpiry <= 0 {
		cfg.DefaultExpiry = 30 * time.Minute
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Manager{
		cfg:      cfg,
		storage:  store,
		roles:    roles,
		notifier: notifier,
		logger:   logger,
	}
}

// NewManager creates a Manager from a config.ApprovalConfig.
// This simplified constructor does not require RoleChecker/Notifier since the
// Checker interface now handles approval messaging directly.
func NewManager(enabled bool, defaultExpiry time.Duration, rules []Rule, store storage.ApprovalRepository, logger *zap.Logger) *Manager {
	cfg := Config{
		Enabled:       enabled,
		DefaultExpiry: defaultExpiry,
		Rules:         rules,
	}
	if cfg.DefaultExpiry <= 0 {
		cfg.DefaultExpiry = 30 * time.Minute
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Manager{
		cfg:     cfg,
		storage: store,
		logger:  logger,
	}
}

// RequiresApproval checks if an action / cluster combination needs approval.
// Returns the matching rule, or nil if none matches.
func (m *Manager) RequiresApproval(action, cluster string) *Rule {
	if !m.cfg.Enabled {
		return nil
	}
	for i := range m.cfg.Rules {
		r := &m.cfg.Rules[i]
		if r.Action != action {
			continue
		}
		for _, c := range r.Clusters {
			if c == "*" || c == cluster {
				return r
			}
		}
	}
	return nil
}

// Submit creates a new pending approval request.
func (m *Manager) Submit(ctx context.Context, req *entity.ApprovalRequest) error {
	rule := m.RequiresApproval(req.Action, req.Cluster)
	if rule == nil {
		return fmt.Errorf("no approval rule matches action %q on cluster %q", req.Action, req.Cluster)
	}

	now := time.Now().UTC()
	req.ID = ulid.Make().String()
	req.Status = entity.ApprovalPending
	req.RequiredApprovals = rule.RequiredApprovals
	req.CreatedAt = now
	req.ExpiresAt = now.Add(m.cfg.DefaultExpiry)

	if err := m.storage.Create(ctx, req); err != nil {
		return fmt.Errorf("creating approval request: %w", err)
	}

	m.logger.Info("approval request created",
		zap.String("id", req.ID),
		zap.String("action", req.Action),
		zap.Int64("requester", req.RequesterID),
	)
	return nil
}

// Decide records an approve/reject decision from a user.
// Returns (needsExecution bool, error).
func (m *Manager) Decide(ctx context.Context, requestID string, approverID int64, approverName, decision, comment string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, err := m.storage.GetByID(ctx, requestID)
	if err != nil {
		return false, fmt.Errorf("fetching approval request: %w", err)
	}
	if req.Status != entity.ApprovalPending {
		return false, fmt.Errorf("request %s is already %s", requestID, req.Status)
	}
	if time.Now().After(req.ExpiresAt) {
		return false, fmt.Errorf("request %s has expired", requestID)
	}
	// Self-approval prevention.
	if approverID == req.RequesterID {
		return false, fmt.Errorf("requester cannot approve their own request")
	}

	now := time.Now().UTC()
	updated := false
	for i := range req.Approvers {
		if req.Approvers[i].UserID == approverID {
			req.Approvers[i].Decision = decision
			req.Approvers[i].Comment = comment
			req.Approvers[i].DecidedAt = &now
			updated = true
			break
		}
	}
	if !updated {
		req.Approvers = append(req.Approvers, entity.Approver{
			UserID:    approverID,
			Decision:  decision,
			Comment:   comment,
			DecidedAt: &now,
		})
	}

	executeNow := false
	if decision == "rejected" || req.HasRejection() {
		req.Status = entity.ApprovalRejected
		req.ResolvedAt = &now
		req.ResolvedBy = &approverID
	} else if req.ApprovalCount() >= req.RequiredApprovals {
		req.Status = entity.ApprovalApproved
		req.ResolvedAt = &now
		req.ResolvedBy = &approverID
		executeNow = true
	}

	if err := m.storage.Update(ctx, req); err != nil {
		return false, fmt.Errorf("updating approval request: %w", err)
	}

	m.logger.Info("approval decision recorded",
		zap.String("id", requestID),
		zap.String("decision", decision),
		zap.Int64("approver", approverID),
	)
	return executeNow, nil
}

// Cancel cancels a pending request by its requester.
func (m *Manager) Cancel(ctx context.Context, requestID string, requesterID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, err := m.storage.GetByID(ctx, requestID)
	if err != nil {
		return err
	}
	if req.RequesterID != requesterID {
		return fmt.Errorf("only the requester can cancel this request")
	}
	if req.Status != entity.ApprovalPending {
		return fmt.Errorf("request is not pending")
	}

	now := time.Now().UTC()
	req.Status = entity.ApprovalCancelled
	req.ResolvedAt = &now
	req.ResolvedBy = &requesterID
	return m.storage.Update(ctx, req)
}

// GetByID returns an approval request by ID.
func (m *Manager) GetByID(ctx context.Context, id string) (*entity.ApprovalRequest, error) {
	return m.storage.GetByID(ctx, id)
}

// ListPending returns all pending requests.
func (m *Manager) ListPending(ctx context.Context) ([]entity.ApprovalRequest, error) {
	return m.storage.ListPending(ctx)
}

// StartExpiryWorker runs a background goroutine that expires timed-out requests.
func (m *Manager) StartExpiryWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := m.storage.ExpireOld(ctx)
				if err != nil {
					m.logger.Error("expiry worker error", zap.Error(err))
					continue
				}
				if n > 0 {
					m.logger.Info("expired approval requests", zap.Int("count", n))
				}
			}
		}
	}()
}
