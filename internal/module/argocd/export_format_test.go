// export_format_test.go exposes pure formatting helpers for white-box testing.
// All functions here are compiled ONLY during `go test`.
package argocd

import (
	"github.com/d9042n/telekube/internal/entity"
	pkgargocd "github.com/d9042n/telekube/pkg/argocd"
)

// ExportedFormatDiffPreview exposes formatDiffPreview for testing.
func ExportedFormatDiffPreview(diff *pkgargocd.DiffResult) string {
	return formatDiffPreview(diff)
}

// ExportedFormatResourceDiff exposes formatResourceDiff for testing.
func ExportedFormatResourceDiff(r pkgargocd.ResourceDiff) string {
	return formatResourceDiff(r)
}

// ExportedComputeSimpleDiff exposes computeSimpleDiff for testing.
func ExportedComputeSimpleDiff(live, target map[string]interface{}) string {
	return computeSimpleDiff(live, target)
}

// ExportedFormatSyncResult exposes formatSyncResult for testing.
func ExportedFormatSyncResult(result *pkgargocd.SyncResult, appName, triggeredBy string) string {
	return formatSyncResult(result, appName, triggeredBy)
}

// ExportedFormatRollbackResult exposes formatRollbackResult for testing.
func ExportedFormatRollbackResult(result *pkgargocd.RollbackResult, appName, revID, triggeredBy string) string {
	return formatRollbackResult(result, appName, revID, triggeredBy)
}

// ExportedFormatAppStatusDetail exposes formatAppStatusDetail for testing.
func ExportedFormatAppStatusDetail(app *pkgargocd.ApplicationStatus, instanceName string) string {
	return formatAppStatusDetail(app, instanceName)
}

// ExportedResourceStatusEmoji exposes resourceStatusEmoji for testing.
func ExportedResourceStatusEmoji(status, health string) string {
	return resourceStatusEmoji(status, health)
}

// ExportedFormatFreezeBlocked exposes formatFreezeBlocked for testing.
func ExportedFormatFreezeBlocked(freeze *entity.DeploymentFreeze) string {
	return formatFreezeBlocked(freeze)
}
