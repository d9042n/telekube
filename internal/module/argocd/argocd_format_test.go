package argocd_test

// argocd_format_test.go — unit tests for pure formatting helpers in the ArgoCD module.
// No telebot instance required. Tests cover all branches of each formatter.

import (
	"strings"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	argocdmod "github.com/d9042n/telekube/internal/module/argocd"
	pkgargocd "github.com/d9042n/telekube/pkg/argocd"
	"github.com/stretchr/testify/assert"
)

// ─── formatDiffPreview ────────────────────────────────────────────────────────

func TestFormatDiffPreview_NoChanges(t *testing.T) {
	t.Parallel()
	diff := &pkgargocd.DiffResult{AppName: "myapp", Changed: 0}
	text := argocdmod.ExportedFormatDiffPreview(diff)
	assert.Contains(t, text, "myapp")
	assert.Contains(t, text, "No pending changes")
}

func TestFormatDiffPreview_WithChanges_ShowsSummary(t *testing.T) {
	t.Parallel()
	diff := &pkgargocd.DiffResult{
		AppName: "myapp",
		Changed: 2,
		Added:   1,
		Removed: 0,
		Resources: []pkgargocd.ResourceDiff{
			{Kind: "Deployment", Name: "api", Diff: "replicas changed"},
		},
	}
	text := argocdmod.ExportedFormatDiffPreview(diff)
	assert.Contains(t, text, "myapp")
	assert.Contains(t, text, "Deployment/api")
	assert.Contains(t, text, "2 changed") // summary line
}

func TestFormatDiffPreview_SkipsEmptyDiffResources(t *testing.T) {
	t.Parallel()
	diff := &pkgargocd.DiffResult{
		AppName: "myapp",
		Changed: 1,
		Resources: []pkgargocd.ResourceDiff{
			{Kind: "Service", Name: "svc", Diff: ""}, // empty diff — should be skipped
		},
	}
	text := argocdmod.ExportedFormatDiffPreview(diff)
	// Empty diff entry should not appear
	assert.NotContains(t, text, "Service/svc")
}

// ─── formatResourceDiff ───────────────────────────────────────────────────────

func TestFormatResourceDiff_Secret_IsRedacted(t *testing.T) {
	t.Parallel()
	r := pkgargocd.ResourceDiff{Kind: "Secret", Name: "mysecret", Diff: "data changed"}
	result := argocdmod.ExportedFormatResourceDiff(r)
	assert.Contains(t, result, "redacted", "Secret diffs must be redacted")
	assert.NotContains(t, result, "data changed", "raw secret diff must not appear")
}

func TestFormatResourceDiff_FallsBackToRawDiff(t *testing.T) {
	t.Parallel()
	r := pkgargocd.ResourceDiff{
		Kind: "Deployment",
		Name: "api",
		Diff: "some diff line",
		Live: "", Target: "", // no JSON → fall through to raw diff
	}
	result := argocdmod.ExportedFormatResourceDiff(r)
	assert.Contains(t, result, "some diff line")
}

func TestFormatResourceDiff_JSONLiveTarget_ComputesSimpleDiff(t *testing.T) {
	t.Parallel()
	r := pkgargocd.ResourceDiff{
		Kind:   "Deployment",
		Name:   "api",
		Diff:   "ignored",
		Live:   `{"replicas": 1}`,
		Target: `{"replicas": 3}`,
	}
	result := argocdmod.ExportedFormatResourceDiff(r)
	assert.Contains(t, result, "replicas", "should show the changed field")
}

// ─── computeSimpleDiff ────────────────────────────────────────────────────────

func TestComputeSimpleDiff_Addition(t *testing.T) {
	t.Parallel()
	live := map[string]interface{}{}
	target := map[string]interface{}{"replicas": float64(3)}
	result := argocdmod.ExportedComputeSimpleDiff(live, target)
	assert.Contains(t, result, "+ replicas")
}

func TestComputeSimpleDiff_Removal(t *testing.T) {
	t.Parallel()
	live := map[string]interface{}{"oldField": "value"}
	target := map[string]interface{}{}
	result := argocdmod.ExportedComputeSimpleDiff(live, target)
	assert.Contains(t, result, "- oldField")
}

func TestComputeSimpleDiff_Changed(t *testing.T) {
	t.Parallel()
	live := map[string]interface{}{"replicas": float64(1)}
	target := map[string]interface{}{"replicas": float64(5)}
	result := argocdmod.ExportedComputeSimpleDiff(live, target)
	assert.Contains(t, result, "~ replicas")
}

func TestComputeSimpleDiff_MaxLinesLimit(t *testing.T) {
	t.Parallel()
	// Adding more than 5 keys — should be truncated with "..."
	target := map[string]interface{}{
		"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6,
	}
	result := argocdmod.ExportedComputeSimpleDiff(map[string]interface{}{}, target)
	assert.Contains(t, result, "...")
}

// ─── formatSyncResult ─────────────────────────────────────────────────────────

func TestFormatSyncResult_Succeeded(t *testing.T) {
	t.Parallel()
	result := &pkgargocd.SyncResult{
		Phase:    "Succeeded",
		Revision: "abc1234567",
		Message:  "application synced",
		Results: []pkgargocd.ResourceResult{
			{Kind: "Deployment", Name: "api", Status: "Synced"},
		},
	}
	text := argocdmod.ExportedFormatSyncResult(result, "myapp", "alice")
	assert.Contains(t, text, "✅")
	assert.Contains(t, text, "myapp")
	assert.Contains(t, text, "Succeeded")
	assert.Contains(t, text, "abc1234") // short rev
	assert.Contains(t, text, "alice")
	assert.Contains(t, text, "Deployment/api")
}

func TestFormatSyncResult_Failed(t *testing.T) {
	t.Parallel()
	result := &pkgargocd.SyncResult{Phase: "Failed"}
	text := argocdmod.ExportedFormatSyncResult(result, "myapp", "bob")
	assert.Contains(t, text, "❌")
}

func TestFormatSyncResult_ResourceFailed(t *testing.T) {
	t.Parallel()
	result := &pkgargocd.SyncResult{
		Phase: "Succeeded",
		Results: []pkgargocd.ResourceResult{
			{Kind: "Deployment", Name: "api", Status: "OutOfSync"},
		},
	}
	text := argocdmod.ExportedFormatSyncResult(result, "app", "user")
	assert.Contains(t, text, "❌") // resource is out of sync
}

func TestFormatSyncResult_EmptyPhase(t *testing.T) {
	t.Parallel()
	result := &pkgargocd.SyncResult{Phase: "", Revision: ""}
	text := argocdmod.ExportedFormatSyncResult(result, "app", "user")
	assert.Contains(t, text, "app")
	// No phase line should appear when phase is empty
	assert.NotContains(t, text, "Status:")
}

// ─── formatRollbackResult ─────────────────────────────────────────────────────

func TestFormatRollbackResult_Success(t *testing.T) {
	t.Parallel()
	result := &pkgargocd.RollbackResult{Phase: "Succeeded"}
	text := argocdmod.ExportedFormatRollbackResult(result, "myapp", "42", "alice")
	assert.Contains(t, text, "✅")
	assert.Contains(t, text, "myapp")
	assert.Contains(t, text, "42")
	assert.Contains(t, text, "alice")
	assert.Contains(t, text, "Auto-sync has been disabled")
}

func TestFormatRollbackResult_Failed(t *testing.T) {
	t.Parallel()
	result := &pkgargocd.RollbackResult{Phase: "Failed", Message: "out of space"}
	text := argocdmod.ExportedFormatRollbackResult(result, "myapp", "41", "bob")
	assert.Contains(t, text, "❌")
	assert.Contains(t, text, "out of space")
}

func TestFormatRollbackResult_NilResult(t *testing.T) {
	t.Parallel()
	// nil result should still produce a readable message
	text := argocdmod.ExportedFormatRollbackResult(nil, "myapp", "39", "eve")
	assert.Contains(t, text, "myapp")
	assert.Contains(t, text, "succeeded") // default phase
}

// ─── formatAppStatusDetail ────────────────────────────────────────────────────

func TestFormatAppStatusDetail_FullInfo(t *testing.T) {
	t.Parallel()
	now := time.Now()
	app := &pkgargocd.ApplicationStatus{
		Name:         "myapp",
		SyncStatus:   "Synced",
		HealthStatus: "Healthy",
		Project:      "default",
		RepoURL:      "https://github.com/org/repo",
		Path:         "deploy/prod",
		TargetRev:    "main",
		CurrentRev:   "abc1234567",
		LastSyncAt:   &now,
		LastSyncBy:   "alice",
		Resources: []pkgargocd.ManagedResource{
			{Kind: "Deployment", Name: "api", Status: "Synced", Health: "Healthy"},
		},
	}
	text := argocdmod.ExportedFormatAppStatusDetail(app, "prod")
	assert.Contains(t, text, "myapp")
	assert.Contains(t, text, "Synced")
	assert.Contains(t, text, "Healthy")
	assert.Contains(t, text, "prod") // instance name
	assert.Contains(t, text, "https://github.com/org/repo")
	assert.Contains(t, text, "deploy/prod")
	assert.Contains(t, text, "abc1234") // short rev
	assert.Contains(t, text, "alice")
	assert.Contains(t, text, "Deployment/api")
}

func TestFormatAppStatusDetail_MinimalInfo(t *testing.T) {
	t.Parallel()
	app := &pkgargocd.ApplicationStatus{
		Name:         "minimal",
		SyncStatus:   "OutOfSync",
		HealthStatus: "Degraded",
	}
	text := argocdmod.ExportedFormatAppStatusDetail(app, "staging")
	assert.Contains(t, text, "minimal")
	assert.Contains(t, text, "OutOfSync")
	// Optional fields should not appear
	assert.NotContains(t, text, "Repo:")
	assert.NotContains(t, text, "Path:")
}

func TestFormatAppStatusDetail_TruncatesAt10Resources(t *testing.T) {
	t.Parallel()
	resources := make([]pkgargocd.ManagedResource, 12)
	for i := range resources {
		resources[i] = pkgargocd.ManagedResource{Kind: "Pod", Name: "p", Status: "Synced", Health: "Healthy"}
	}
	app := &pkgargocd.ApplicationStatus{Name: "app", Resources: resources}
	text := argocdmod.ExportedFormatAppStatusDetail(app, "prod")
	assert.Contains(t, text, "and 2 more")
}

// ─── resourceStatusEmoji ──────────────────────────────────────────────────────

func TestResourceStatusEmoji(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status string
		health string
		want   string
	}{
		{"OutOfSync", "Healthy", "🟡"},
		{"Synced", "Degraded", "🔴"},
		{"Synced", "Missing", "🔴"},
		{"Synced", "Healthy", "✅"},
		// Progressing is not Degraded/Missing, and status is Synced → returns ✅
		{"Synced", "Progressing", "✅"},
		// Neither OutOfSync nor Synced → returns ⚪
		{"Unknown", "Unknown", "⚪"},
		{"Unknown", "Healthy", "⚪"},
	}
	for _, tt := range tests {
		t.Run(tt.status+"/"+tt.health, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, argocdmod.ExportedResourceStatusEmoji(tt.status, tt.health))
		})
	}
}

// ─── formatFreezeBlocked ─────────────────────────────────────────────────────

func TestFormatFreezeBlocked_AllClusters(t *testing.T) {
	t.Parallel()
	freeze := &entity.DeploymentFreeze{
		Scope:     "all",
		Reason:    "maintenance",
		ExpiresAt: time.Now().Add(2 * time.Hour),
	}
	text := argocdmod.ExportedFormatFreezeBlocked(freeze)
	assert.Contains(t, text, "Deployment Freeze Active")
	assert.Contains(t, text, "all clusters", "scope 'all' should display as 'all clusters'")
}

func TestFormatFreezeBlocked_SpecificCluster(t *testing.T) {
	t.Parallel()
	freeze := &entity.DeploymentFreeze{
		Scope:     "prod",
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	text := argocdmod.ExportedFormatFreezeBlocked(freeze)
	assert.Contains(t, text, "prod")
	assert.True(t, strings.Contains(text, "BLOCKED"), "must indicate operation is blocked")
}
