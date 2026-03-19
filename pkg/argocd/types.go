// Package argocd provides a production-grade HTTP client for the ArgoCD REST API v2.
package argocd

import (
	"context"
	"time"
)

// ListOpts are optional filters for listing applications.
type ListOpts struct {
	Projects []string
	Selector string
}

// SyncOpts control the sync behaviour.
type SyncOpts struct {
	Prune  bool
	Force  bool
	DryRun bool
}

// Application represents an ArgoCD Application resource.
type Application struct {
	Name         string     `json:"name"`
	Namespace    string     `json:"namespace"`
	Project      string     `json:"project"`
	RepoURL      string     `json:"repoURL"`
	Path         string     `json:"path"`
	TargetRev    string     `json:"targetRevision"`
	SyncStatus   string     `json:"syncStatus"`
	HealthStatus string     `json:"healthStatus"`
	CurrentRev   string     `json:"currentRevision"`
	LastSyncAt   *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncBy   string     `json:"lastSyncBy,omitempty"`
}

// ApplicationStatus holds the detailed status of an application.
type ApplicationStatus struct {
	Name         string            `json:"name"`
	SyncStatus   string            `json:"syncStatus"`
	HealthStatus string            `json:"healthStatus"`
	Project      string            `json:"project"`
	RepoURL      string            `json:"repoURL"`
	Path         string            `json:"path"`
	TargetRev    string            `json:"targetRevision"`
	CurrentRev   string            `json:"currentRevision"`
	Resources    []ManagedResource `json:"resources"`
	LastSyncAt   *time.Time        `json:"lastSyncAt,omitempty"`
	LastSyncBy   string            `json:"lastSyncBy,omitempty"`
	Message      string            `json:"message,omitempty"`
}

// ManagedResource represents a single Kubernetes resource managed by ArgoCD.
type ManagedResource struct {
	Kind                string `json:"kind"`
	Name                string `json:"name"`
	Namespace           string `json:"namespace"`
	Group               string `json:"group"`
	Version             string `json:"version"`
	Status              string `json:"status"`
	Health              string `json:"health"`
	NormalizedLiveState string `json:"normalizedLiveState,omitempty"`
	PredictedLiveState  string `json:"predictedLiveState,omitempty"`
}

// SyncResult is the outcome of a sync operation.
type SyncResult struct {
	Phase    string           `json:"phase"`
	Message  string           `json:"message"`
	Revision string           `json:"revision"`
	Results  []ResourceResult `json:"results"`
}

// ResourceResult is the per-resource outcome during sync.
type ResourceResult struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// RollbackResult is the outcome of a rollback operation.
type RollbackResult struct {
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

// RevisionHistory is a historical deployment entry.
type RevisionHistory struct {
	ID         int64     `json:"id"`
	Revision   string    `json:"revision"`
	DeployedAt time.Time `json:"deployedAt"`
	DeployedBy string    `json:"initiatedBy,omitempty"`
	Source     struct {
		RepoURL        string `json:"repoURL"`
		Path           string `json:"path"`
		TargetRevision string `json:"targetRevision"`
	} `json:"source"`
}

// DiffResult aggregates pending diffs for an application.
type DiffResult struct {
	AppName   string         `json:"appName"`
	Changed   int            `json:"changed"`
	Added     int            `json:"added"`
	Removed   int            `json:"removed"`
	Resources []ResourceDiff `json:"resources"`
}

// ResourceDiff holds the diff for a single resource.
type ResourceDiff struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Diff      string `json:"diff"`
	Live      string `json:"live,omitempty"`   // live state JSON
	Target    string `json:"target,omitempty"` // desired state JSON
}

// Client is the interface for the ArgoCD API client.
type Client interface {
	// Applications
	ListApplications(ctx context.Context, opts ListOpts) ([]Application, error)
	GetApplication(ctx context.Context, name string) (*Application, error)
	GetApplicationStatus(ctx context.Context, name string) (*ApplicationStatus, error)

	// Sync
	SyncApplication(ctx context.Context, name string, opts SyncOpts) (*SyncResult, error)

	// Rollback
	RollbackApplication(ctx context.Context, name string, revision int64) (*RollbackResult, error)
	GetApplicationHistory(ctx context.Context, name string) ([]RevisionHistory, error)

	// Diff
	GetManagedResources(ctx context.Context, name string) ([]ManagedResource, error)
	GetApplicationDiff(ctx context.Context, name string) (*DiffResult, error)

	// Auto-sync management
	DisableAutoSync(ctx context.Context, name string) error
	EnableAutoSync(ctx context.Context, name string) error

	// Health
	Ping(ctx context.Context) error
}
