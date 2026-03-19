// Package entity_test provides unit tests for domain model logic.
package entity_test

import (
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/stretchr/testify/assert"
)

// ─── PolicyRule.Matches ───────────────────────────────────────────────────────

func TestPolicyRule_Matches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rule      entity.PolicyRule
		module    string
		resource  string
		action    string
		cluster   string
		namespace string
		want      bool
	}{
		{
			name: "wildcard all fields - matches anything",
			rule: entity.PolicyRule{
				Modules:    []string{"*"},
				Resources:  []string{"*"},
				Actions:    []string{"*"},
				Clusters:   []string{"*"},
				Namespaces: []string{"*"},
				Effect:     "allow",
			},
			module: "kubernetes", resource: "pods", action: "list",
			cluster: "prod-1", namespace: "default",
			want: true,
		},
		{
			name: "exact match all fields",
			rule: entity.PolicyRule{
				Modules:    []string{"kubernetes"},
				Resources:  []string{"pods"},
				Actions:    []string{"restart"},
				Clusters:   []string{"prod-1"},
				Namespaces: []string{"production"},
				Effect:     "allow",
			},
			module: "kubernetes", resource: "pods", action: "restart",
			cluster: "prod-1", namespace: "production",
			want: true,
		},
		{
			name: "module mismatch",
			rule: entity.PolicyRule{
				Modules:    []string{"argocd"},
				Resources:  []string{"*"},
				Actions:    []string{"*"},
				Clusters:   []string{"*"},
				Namespaces: []string{"*"},
				Effect:     "allow",
			},
			module: "kubernetes", resource: "pods", action: "list",
			cluster: "prod-1", namespace: "default",
			want: false,
		},
		{
			name: "action mismatch",
			rule: entity.PolicyRule{
				Modules:    []string{"kubernetes"},
				Resources:  []string{"pods"},
				Actions:    []string{"list"},
				Clusters:   []string{"*"},
				Namespaces: []string{"*"},
				Effect:     "allow",
			},
			module: "kubernetes", resource: "pods", action: "delete",
			cluster: "prod-1", namespace: "default",
			want: false,
		},
		{
			name: "cluster mismatch - prod rule vs staging request",
			rule: entity.PolicyRule{
				Modules:    []string{"*"},
				Resources:  []string{"*"},
				Actions:    []string{"*"},
				Clusters:   []string{"prod-1"},
				Namespaces: []string{"*"},
				Effect:     "allow",
			},
			module: "kubernetes", resource: "pods", action: "list",
			cluster: "staging", namespace: "default",
			want: false,
		},
		{
			name: "namespace mismatch",
			rule: entity.PolicyRule{
				Modules:    []string{"*"},
				Resources:  []string{"*"},
				Actions:    []string{"*"},
				Clusters:   []string{"*"},
				Namespaces: []string{"production"},
				Effect:     "allow",
			},
			module: "kubernetes", resource: "pods", action: "list",
			cluster: "prod-1", namespace: "staging",
			want: false,
		},
		{
			name: "multiple modules - one matches",
			rule: entity.PolicyRule{
				Modules:    []string{"kubernetes", "argocd"},
				Resources:  []string{"*"},
				Actions:    []string{"*"},
				Clusters:   []string{"*"},
				Namespaces: []string{"*"},
				Effect:     "allow",
			},
			module: "argocd", resource: "apps", action: "sync",
			cluster: "prod-1", namespace: "default",
			want: true,
		},
		{
			name: "empty modules list - matches nothing",
			rule: entity.PolicyRule{
				Modules:    []string{},
				Resources:  []string{"*"},
				Actions:    []string{"*"},
				Clusters:   []string{"*"},
				Namespaces: []string{"*"},
				Effect:     "allow",
			},
			module: "kubernetes", resource: "pods", action: "list",
			cluster: "prod-1", namespace: "default",
			want: false,
		},
		{
			name: "wildcard in actions only",
			rule: entity.PolicyRule{
				Modules:    []string{"kubernetes"},
				Resources:  []string{"pods"},
				Actions:    []string{"*"},
				Clusters:   []string{"*"},
				Namespaces: []string{"*"},
				Effect:     "deny",
			},
			module: "kubernetes", resource: "pods", action: "delete",
			cluster: "any", namespace: "any",
			want: true,
		},
		{
			name: "partial wildcard not supported - literal match only",
			rule: entity.PolicyRule{
				Modules:    []string{"kube*"},
				Resources:  []string{"*"},
				Actions:    []string{"*"},
				Clusters:   []string{"*"},
				Namespaces: []string{"*"},
				Effect:     "allow",
			},
			module: "kubernetes", resource: "pods", action: "list",
			cluster: "prod-1", namespace: "default",
			want: false, // "kube*" is not the same as "kubernetes"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.rule.Matches(tt.module, tt.resource, tt.action, tt.cluster, tt.namespace)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ─── UserRoleBinding.IsExpired ────────────────────────────────────────────────

func TestUserRoleBinding_IsExpired(t *testing.T) {
	t.Parallel()

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	tests := []struct {
		name    string
		binding entity.UserRoleBinding
		want    bool
	}{
		{
			name: "nil expiry - never expires",
			binding: entity.UserRoleBinding{
				UserID:    1,
				RoleName:  "admin",
				ExpiresAt: nil,
			},
			want: false,
		},
		{
			name: "past expiry - is expired",
			binding: entity.UserRoleBinding{
				UserID:    1,
				RoleName:  "admin",
				ExpiresAt: &past,
			},
			want: true,
		},
		{
			name: "future expiry - not expired",
			binding: entity.UserRoleBinding{
				UserID:    1,
				RoleName:  "admin",
				ExpiresAt: &future,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.binding.IsExpired())
		})
	}
}

// ─── ApprovalRequest state machine ───────────────────────────────────────────

func TestApprovalRequest_IsPending(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status entity.ApprovalStatus
		want   bool
	}{
		{"pending", entity.ApprovalPending, true},
		{"approved", entity.ApprovalApproved, false},
		{"rejected", entity.ApprovalRejected, false},
		{"expired", entity.ApprovalExpired, false},
		{"cancelled", entity.ApprovalCancelled, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := &entity.ApprovalRequest{Status: tt.status}
			assert.Equal(t, tt.want, req.IsPending())
		})
	}
}

func TestApprovalRequest_ApprovalCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		approvers []entity.Approver
		want      int
	}{
		{
			name:      "no approvers",
			approvers: nil,
			want:      0,
		},
		{
			name: "all pending",
			approvers: []entity.Approver{
				{UserID: 1, Decision: "pending"},
				{UserID: 2, Decision: "pending"},
			},
			want: 0,
		},
		{
			name: "one approved",
			approvers: []entity.Approver{
				{UserID: 1, Decision: "approved"},
				{UserID: 2, Decision: "pending"},
			},
			want: 1,
		},
		{
			name: "two approved, one rejected",
			approvers: []entity.Approver{
				{UserID: 1, Decision: "approved"},
				{UserID: 2, Decision: "approved"},
				{UserID: 3, Decision: "rejected"},
			},
			want: 2,
		},
		{
			name: "all approved",
			approvers: []entity.Approver{
				{UserID: 1, Decision: "approved"},
				{UserID: 2, Decision: "approved"},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := &entity.ApprovalRequest{Approvers: tt.approvers}
			assert.Equal(t, tt.want, req.ApprovalCount())
		})
	}
}

func TestApprovalRequest_HasRejection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		approvers []entity.Approver
		want      bool
	}{
		{
			name:      "no approvers",
			approvers: nil,
			want:      false,
		},
		{
			name: "all approved",
			approvers: []entity.Approver{
				{UserID: 1, Decision: "approved"},
			},
			want: false,
		},
		{
			name: "one rejection among approvals",
			approvers: []entity.Approver{
				{UserID: 1, Decision: "approved"},
				{UserID: 2, Decision: "rejected"},
			},
			want: true,
		},
		{
			name: "only rejection",
			approvers: []entity.Approver{
				{UserID: 1, Decision: "rejected"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := &entity.ApprovalRequest{Approvers: tt.approvers}
			assert.Equal(t, tt.want, req.HasRejection())
		})
	}
}

// ─── IsValidRole ──────────────────────────────────────────────────────────────

func TestIsValidRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		role string
		want bool
	}{
		{"admin is valid", entity.RoleAdmin, true},
		{"operator is valid", entity.RoleOperator, true},
		{"viewer is valid", entity.RoleViewer, true},
		{"super-admin is valid", entity.RoleSuperAdmin, true},
		{"on-call is valid", entity.RoleOnCall, true},
		{"empty string is invalid", "", false},
		{"arbitrary string is invalid", "superuser", false},
		{"partial match is invalid", "adm", false},
		{"case sensitive - uppercase invalid", "Admin", false},
		{"space padded is invalid", " admin", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, entity.IsValidRole(tt.role))
		})
	}
}

func TestValidRoles(t *testing.T) {
	t.Parallel()
	roles := entity.ValidRoles()
	assert.Len(t, roles, 5)
	assert.Contains(t, roles, entity.RoleSuperAdmin)
	assert.Contains(t, roles, entity.RoleAdmin)
	assert.Contains(t, roles, entity.RoleOperator)
	assert.Contains(t, roles, entity.RoleViewer)
	assert.Contains(t, roles, entity.RoleOnCall)
}

// ─── HealthStatus.Emoji() ─────────────────────────────────────────────────────

func TestHealthStatusEmoji(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status entity.HealthStatus
		want   string
	}{
		{entity.HealthStatusHealthy, "🟢"},
		{entity.HealthStatusUnhealthy, "🔴"},
		{entity.HealthStatusUnknown, "⚪"},
		{"", "⚪"},        // empty → default
		{"anything", "⚪"}, // unknown value → default
	}

	for _, tt := range tests {
		t.Run(string(tt.status)+"_emoji", func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.status.Emoji())
		})
	}
}

// ─── DeploymentFreeze.IsActive() ─────────────────────────────────────────────

func TestDeploymentFreezeIsActive(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name     string
		freeze   entity.DeploymentFreeze
		expected bool
	}{
		{
			name:     "active — not expired, not thawed",
			freeze:   entity.DeploymentFreeze{ExpiresAt: now.Add(1 * time.Hour), ThawedAt: nil},
			expected: true,
		},
		{
			name:     "expired — ExpiresAt in the past",
			freeze:   entity.DeploymentFreeze{ExpiresAt: now.Add(-1 * time.Millisecond), ThawedAt: nil},
			expected: false,
		},
		{
			name:     "thawed — ThawedAt set even if not expired",
			freeze:   entity.DeploymentFreeze{ExpiresAt: now.Add(1 * time.Hour), ThawedAt: &now},
			expected: false,
		},
		{
			name:     "both thawed and expired",
			freeze:   entity.DeploymentFreeze{ExpiresAt: now.Add(-1 * time.Hour), ThawedAt: &now},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.freeze.IsActive())
		})
	}
}

// ─── DeploymentFreeze.RemainingDuration() ─────────────────────────────────────

func TestDeploymentFreezeRemainingDuration(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name        string
		freeze      entity.DeploymentFreeze
		wantZero    bool
		wantAtLeast time.Duration
	}{
		{
			name:        "active freeze — remaining is positive",
			freeze:      entity.DeploymentFreeze{ExpiresAt: now.Add(30 * time.Minute), ThawedAt: nil},
			wantZero:    false,
			wantAtLeast: 29 * time.Minute,
		},
		{
			name:     "expired freeze — remaining is 0",
			freeze:   entity.DeploymentFreeze{ExpiresAt: now.Add(-1 * time.Hour), ThawedAt: nil},
			wantZero: true,
		},
		{
			name:     "thawed freeze — remaining is 0",
			freeze:   entity.DeploymentFreeze{ExpiresAt: now.Add(1 * time.Hour), ThawedAt: &now},
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			remaining := tt.freeze.RemainingDuration()
			if tt.wantZero {
				assert.Equal(t, time.Duration(0), remaining)
			} else {
				assert.GreaterOrEqual(t, remaining, tt.wantAtLeast)
			}
		})
	}
}

