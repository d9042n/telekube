// export_test.go exposes internal helpers from the argocd package for
// black-box testing from the argocd_test package.
// This file is compiled ONLY during `go test`.
package argocd

import (
	"context"

	pkgargocd "github.com/d9042n/telekube/pkg/argocd"
	"gopkg.in/telebot.v3"
)

// ExportedListApplications exposes the internal sendAppList call for testing.
// It calls the ArgoCD client directly (bypassing handler + RBAC).
func (m *Module) ExportedListApplications(ctx context.Context, instanceName string) ([]pkgargocd.Application, error) {
	inst, err := m.getInstanceByName(instanceName)
	if err != nil {
		return nil, err
	}
	return inst.client.ListApplications(ctx, pkgargocd.ListOpts{})
}

// ExportedGetApplicationHistory exposes internal history retrieval.
func (m *Module) ExportedGetApplicationHistory(ctx context.Context, instanceName, appName string) ([]pkgargocd.RevisionHistory, error) {
	inst, err := m.getInstanceByName(instanceName)
	if err != nil {
		return nil, err
	}
	return inst.client.GetApplicationHistory(ctx, appName)
}

// ExportedGetApplication exposes internal single-app retrieval.
func (m *Module) ExportedGetApplication(ctx context.Context, instanceName, appName string) (*pkgargocd.Application, error) {
	inst, err := m.getInstanceByName(instanceName)
	if err != nil {
		return nil, err
	}
	status, err := inst.client.GetApplicationStatus(ctx, appName)
	if err != nil {
		return nil, err
	}
	return &pkgargocd.Application{
		Name:         status.Name,
		SyncStatus:   status.SyncStatus,
		HealthStatus: status.HealthStatus,
	}, nil
}

// ExportedGetInstanceByName exposes getInstanceByName for testing.
func (m *Module) ExportedGetInstanceByName(name string) (*instanceClient, error) {
	return m.getInstanceByName(name)
}

// ExportedGetDefaultInstance exposes getDefaultInstance for testing.
func (m *Module) ExportedGetDefaultInstance() (*instanceClient, error) {
	return m.getDefaultInstance()
}

// ExportedSyncStatusEmoji exposes the package-level syncStatusEmoji function.
func ExportedSyncStatusEmoji(syncStatus, healthStatus string) string {
	return syncStatusEmoji(syncStatus, healthStatus)
}

// ExportedShortRev exposes the shortRev helper.
func ExportedShortRev(rev string) string { return shortRev(rev) }

// ExportedFormatRollbackHistory exposes the formatRollbackHistory helper.
func ExportedFormatRollbackHistory(history []pkgargocd.RevisionHistory, instanceName, appName string) (string, *telebot.ReplyMarkup) {
	return formatRollbackHistory(history, instanceName, appName)
}

