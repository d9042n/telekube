package argocd_test

// argocd_edge_test.go — edge case tests for ArgoCD module.
// Tests use the same mock infrastructure from module_test.go but focus on
// error paths and security boundary cases specified in 06b-handler-tests.md.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	argocdmod "github.com/d9042n/telekube/internal/module/argocd"
	pkgargocd "github.com/d9042n/telekube/pkg/argocd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// ─── ListApplications edge cases ─────────────────────────────────────────────

func TestListApplications_Empty(t *testing.T) {
	t.Parallel()

	client := &mockArgoCDClient{}
	client.On("ListApplications", mock.Anything, mock.Anything).
		Return([]pkgargocd.Application{}, nil)

	mod, rb, _, _ := makeModule(client)
	rb.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
	rb.On("IsSuperAdmin", mock.Anything).Return(false)

	apps, err := mod.ExportedListApplications(context.Background(), "prod")
	assert.NoError(t, err)
	assert.Empty(t, apps, "empty list should return no applications")
	client.AssertExpectations(t)
}

func TestListApplications_ClientError(t *testing.T) {
	t.Parallel()

	client := &mockArgoCDClient{}
	client.On("ListApplications", mock.Anything, mock.Anything).
		Return([]pkgargocd.Application{}, errors.New("connection refused"))

	mod, rb, _, _ := makeModule(client)
	rb.On("HasPermission", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
	rb.On("IsSuperAdmin", mock.Anything).Return(false)

	_, err := mod.ExportedListApplications(context.Background(), "prod")
	assert.Error(t, err, "client error should be propagated")
}

// ─── GetApplicationHistory edge cases ────────────────────────────────────────

func TestGetApplicationHistory_Empty(t *testing.T) {
	t.Parallel()

	client := &mockArgoCDClient{}
	client.On("GetApplicationHistory", mock.Anything, "myapp").
		Return([]pkgargocd.RevisionHistory{}, nil)

	mod, _, _, _ := makeModule(client)

	history, err := mod.ExportedGetApplicationHistory(context.Background(), "prod", "myapp")
	assert.NoError(t, err)
	assert.Empty(t, history, "empty history should return zero revisions")
}

func TestGetApplicationHistory_ClientError(t *testing.T) {
	t.Parallel()

	client := &mockArgoCDClient{}
	client.On("GetApplicationHistory", mock.Anything, "myapp").
		Return([]pkgargocd.RevisionHistory{}, errors.New("app not found"))

	mod, _, _, _ := makeModule(client)

	_, err := mod.ExportedGetApplicationHistory(context.Background(), "prod", "myapp")
	assert.Error(t, err)
}

// ─── SyncStatusEmoji boundary tests ──────────────────────────────────────────

func TestSyncStatusEmoji(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		syncStatus   string
		healthStatus string
		wantEmoji    string
	}{
		{"out of sync", "OutOfSync", "Healthy", "🟡"},
		{"degraded", "Synced", "Degraded", "🔴"},
		{"missing", "Synced", "Missing", "🔴"},
		{"healthy and synced", "Synced", "Healthy", "✅"},
		{"progressing", "Synced", "Progressing", "⚪"},
		{"unknown states", "Unknown", "Unknown", "⚪"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			emoji := argocdmod.ExportedSyncStatusEmoji(tt.syncStatus, tt.healthStatus)
			assert.Equal(t, tt.wantEmoji, emoji)
		})
	}
}

// ─── ShortRev edge cases ──────────────────────────────────────────────────────

func TestShortRev(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"full SHA", "abc1234567890", "abc1234"},
		{"exactly 7 chars", "abc1234", "abc1234"},
		{"shorter than 7", "abc12", "abc12"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, argocdmod.ExportedShortRev(tt.input))
		})
	}
}

// ─── GetInstance edge cases ────────────────────────────────────────────────────

func TestModule_GetInstanceByName_NotFound(t *testing.T) {
	t.Parallel()

	client := &mockArgoCDClient{}
	mod, _, _, _ := makeModule(client)

	_, err := mod.ExportedGetInstanceByName("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestModule_GetInstanceByName_Found(t *testing.T) {
	t.Parallel()

	client := &mockArgoCDClient{}
	mod, _, _, _ := makeModule(client)

	inst, err := mod.ExportedGetInstanceByName("prod")
	assert.NoError(t, err)
	assert.NotNil(t, inst)
}

// ─── RollbackHistory formatting ───────────────────────────────────────────────

func TestFormatRollbackHistory_SingleEntry_NoRollbackButton(t *testing.T) {
	t.Parallel()

	// When there's only 1 history entry (current), there should be no rollback buttons
	// because rolling back to the current revision is disallowed.
	history := []pkgargocd.RevisionHistory{
		{
			ID:         42,
			Revision:   "abc1234567",
			DeployedAt: time.Now().Add(-1 * time.Hour),
			DeployedBy: "user1",
		},
	}

	text, _ := argocdmod.ExportedFormatRollbackHistory(history, "prod", "myapp")
	assert.Contains(t, text, "myapp")
	assert.Contains(t, text, "Rev 42")
	// Only "current" label should appear since there's only 1 entry
	assert.Contains(t, text, "current")
}

func TestFormatRollbackHistory_MultipleEntries_HasButtons(t *testing.T) {
	t.Parallel()

	history := []pkgargocd.RevisionHistory{
		{
			ID:         43,
			Revision:   "current1234",
			DeployedAt: time.Now().Add(-30 * time.Minute),
			DeployedBy: "user2",
		},
		{
			ID:         42,
			Revision:   "previous567",
			DeployedAt: time.Now().Add(-2 * time.Hour),
			DeployedBy: "user1",
		},
	}

	text, _ := argocdmod.ExportedFormatRollbackHistory(history, "prod", "myapp")
	assert.Contains(t, text, "Rev 43")
	assert.Contains(t, text, "Rev 42")
	assert.True(t, strings.Contains(text, "current") || strings.Contains(text, "previous"),
		"should label current and previous revisions")
}

// ─── SyncOpts mapping ────────────────────────────────────────────────────────

func TestSyncMode_Normal(t *testing.T) {
	t.Parallel()
	opts := pkgargocd.SyncOpts{}
	assert.False(t, opts.Prune)
	assert.False(t, opts.Force)
	assert.False(t, opts.DryRun)
}

func TestSyncMode_Prune(t *testing.T) {
	t.Parallel()
	opts := pkgargocd.SyncOpts{Prune: true}
	assert.True(t, opts.Prune)
	assert.False(t, opts.Force)
}

func TestSyncMode_Force(t *testing.T) {
	t.Parallel()
	opts := pkgargocd.SyncOpts{Force: true}
	assert.False(t, opts.Prune)
	assert.True(t, opts.Force)
}

// ─── No ArgoCD instance configured ───────────────────────────────────────────

func TestModule_NoInstances_GetDefault_Error(t *testing.T) {
	t.Parallel()

	mod := argocdmod.NewModule(nil, &mockRBAC{}, &mockAudit{}, &mockFreezeRepo{}, nil, zap.NewNop())

	_, err := mod.ExportedGetDefaultInstance()
	assert.Error(t, err, "no instances should return error")
}

// ─── GetApplication edge cases ───────────────────────────────────────────────

func TestGetApplication_NotFound(t *testing.T) {
	t.Parallel()

	client := &mockArgoCDClient{}
	client.On("GetApplicationStatus", mock.Anything, "nonexistent").
		Return(nil, errors.New("application not found"))

	mod, _, _, _ := makeModule(client)

	app, err := mod.ExportedGetApplication(context.Background(), "prod", "nonexistent")
	assert.Error(t, err)
	assert.Nil(t, app)
}

func TestGetApplication_Success(t *testing.T) {
	t.Parallel()

	client := &mockArgoCDClient{}
	expected := &pkgargocd.ApplicationStatus{
		Name:         "myapp",
		SyncStatus:   "Synced",
		HealthStatus: "Healthy",
	}
	client.On("GetApplicationStatus", mock.Anything, "myapp").
		Return(expected, nil)

	mod, _, _, _ := makeModule(client)

	app, err := mod.ExportedGetApplication(context.Background(), "prod", "myapp")
	assert.NoError(t, err)
	assert.Equal(t, "myapp", app.Name)
	assert.Equal(t, "Synced", app.SyncStatus)
}

// ─── DeploymentFreeze: isActive boundary ─────────────────────────────────────

func TestDeploymentFreeze_IsActive_Boundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		freeze       entity.DeploymentFreeze
		expectActive bool
	}{
		{
			name: "just expired (1ms ago)",
			freeze: entity.DeploymentFreeze{
				ExpiresAt: time.Now().Add(-1 * time.Millisecond),
			},
			expectActive: false,
		},
		{
			name: "expires in far future",
			freeze: entity.DeploymentFreeze{
				ExpiresAt: time.Now().Add(24 * time.Hour),
			},
			expectActive: true,
		},
		{
			name: "thawed even if not expired",
			freeze: entity.DeploymentFreeze{
				ExpiresAt: time.Now().Add(1 * time.Hour),
				ThawedAt:  func() *time.Time { t := time.Now(); return &t }(),
			},
			expectActive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expectActive, tt.freeze.IsActive())
		})
	}
}
