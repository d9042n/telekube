package argocd

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 3
	defaultBackoffBase = 300 * time.Millisecond
)

// ClientConfig holds connection settings for a single ArgoCD instance.
type ClientConfig struct {
	BaseURL  string
	Auth     AuthProvider
	Insecure bool
	CACert   []byte
	Timeout  time.Duration
	Logger   *zap.Logger
}

type httpClient struct {
	baseURL    string
	auth       AuthProvider
	http       *http.Client
	maxRetries int
	logger     *zap.Logger
}

// NewClient creates a new ArgoCD HTTP client.
func NewClient(cfg ClientConfig) Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.Insecure, //nolint:gosec
	}

	transport := &http.Transport{
		TLSClientConfig: tlsCfg,
	}

	return &httpClient{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		auth:    cfg.Auth,
		http: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		maxRetries: defaultMaxRetries,
		logger:     cfg.Logger,
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (c *httpClient) doJSON(ctx context.Context, method, path string, body, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	var lastErr error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := defaultBackoffBase * (1 << (attempt - 1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			// Reset reader for retries
			if body != nil {
				b, _ := json.Marshal(body)
				reqBody = bytes.NewReader(b)
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		if c.auth != nil {
			if err := c.auth.Apply(req); err != nil {
				return fmt.Errorf("applying auth: %w", err)
			}
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("executing request: %w", err)
			c.logger.Warn("argocd request failed, retrying",
				zap.String("method", method),
				zap.String("path", path),
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
			continue
		}
		defer func() { _ = resp.Body.Close() }()

		respBody, _ := io.ReadAll(resp.Body)

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("argocd server error %d: %s", resp.StatusCode, respBody)
			continue // retry on 5xx
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("argocd client error %d: %s", resp.StatusCode, respBody)
		}

		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("decoding response: %w", err)
			}
		}
		return nil
	}
	return fmt.Errorf("argocd request failed after %d attempts: %w", c.maxRetries, lastErr)
}

// ─── Applications ──────────────────────────────────────────────────────────

// argoCDAppListResponse mirrors the ArgoCD API response for app list.
type argoCDAppListResponse struct {
	Items []argoCDApp `json:"items"`
}

type argoCDApp struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Project string `json:"project"`
		Source  struct {
			RepoURL        string `json:"repoURL"`
			Path           string `json:"path"`
			TargetRevision string `json:"targetRevision"`
		} `json:"source"`
	} `json:"spec"`
	Status struct {
		Sync struct {
			Status   string `json:"status"`
			Revision string `json:"revision"`
		} `json:"sync"`
		Health struct {
			Status string `json:"status"`
		} `json:"health"`
		OperationState *struct {
			StartedAt time.Time `json:"startedAt"`
			InitiatedBy struct {
				Username string `json:"username"`
			} `json:"initiatedBy"`
		} `json:"operationState,omitempty"`
		Resources []struct {
			Kind      string `json:"kind"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Group     string `json:"group"`
			Version   string `json:"version"`
			Status    string `json:"status"`
			Health    *struct {
				Status string `json:"status"`
			} `json:"health,omitempty"`
		} `json:"resources"`
		History []struct {
			ID         int64     `json:"id"`
			Revision   string    `json:"revision"`
			DeployedAt time.Time `json:"deployedAt"`
			Source     struct {
				RepoURL        string `json:"repoURL"`
				Path           string `json:"path"`
				TargetRevision string `json:"targetRevision"`
			} `json:"source"`
			InitiatedBy *struct {
				Username string `json:"username"`
			} `json:"initiatedBy,omitempty"`
		} `json:"history"`
	} `json:"status"`
}

func appFromRaw(raw argoCDApp) Application {
	app := Application{
		Name:         raw.Metadata.Name,
		Namespace:    raw.Metadata.Namespace,
		Project:      raw.Spec.Project,
		RepoURL:      raw.Spec.Source.RepoURL,
		Path:         raw.Spec.Source.Path,
		TargetRev:    raw.Spec.Source.TargetRevision,
		SyncStatus:   raw.Status.Sync.Status,
		HealthStatus: raw.Status.Health.Status,
		CurrentRev:   raw.Status.Sync.Revision,
	}
	if raw.Status.OperationState != nil {
		t := raw.Status.OperationState.StartedAt
		app.LastSyncAt = &t
		app.LastSyncBy = raw.Status.OperationState.InitiatedBy.Username
	}
	return app
}

func (c *httpClient) ListApplications(ctx context.Context, opts ListOpts) ([]Application, error) {
	var raw argoCDAppListResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/applications", nil, &raw); err != nil {
		return nil, fmt.Errorf("listing applications: %w", err)
	}
	apps := make([]Application, 0, len(raw.Items))
	for _, item := range raw.Items {
		apps = append(apps, appFromRaw(item))
	}
	return apps, nil
}

func (c *httpClient) GetApplication(ctx context.Context, name string) (*Application, error) {
	var raw argoCDApp
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/applications/"+name, nil, &raw); err != nil {
		return nil, fmt.Errorf("getting application %q: %w", name, err)
	}
	app := appFromRaw(raw)
	return &app, nil
}

func (c *httpClient) GetApplicationStatus(ctx context.Context, name string) (*ApplicationStatus, error) {
	var raw argoCDApp
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/applications/"+name, nil, &raw); err != nil {
		return nil, fmt.Errorf("getting application status %q: %w", name, err)
	}
	status := &ApplicationStatus{
		Name:         raw.Metadata.Name,
		SyncStatus:   raw.Status.Sync.Status,
		HealthStatus: raw.Status.Health.Status,
		Project:      raw.Spec.Project,
		RepoURL:      raw.Spec.Source.RepoURL,
		Path:         raw.Spec.Source.Path,
		TargetRev:    raw.Spec.Source.TargetRevision,
		CurrentRev:   raw.Status.Sync.Revision,
	}
	for _, r := range raw.Status.Resources {
		mr := ManagedResource{
			Kind:      r.Kind,
			Name:      r.Name,
			Namespace: r.Namespace,
			Group:     r.Group,
			Version:   r.Version,
			Status:    r.Status,
		}
		if r.Health != nil {
			mr.Health = r.Health.Status
		}
		status.Resources = append(status.Resources, mr)
	}
	if raw.Status.OperationState != nil {
		t := raw.Status.OperationState.StartedAt
		status.LastSyncAt = &t
		status.LastSyncBy = raw.Status.OperationState.InitiatedBy.Username
	}
	return status, nil
}

// ─── Sync ─────────────────────────────────────────────────────────────────

func (c *httpClient) SyncApplication(ctx context.Context, name string, opts SyncOpts) (*SyncResult, error) {
	payload := map[string]interface{}{
		"prune": opts.Prune,
		"dryRun": opts.DryRun,
	}
	if opts.Force {
		payload["strategy"] = map[string]interface{}{
			"apply": map[string]interface{}{"force": true},
		}
	}

	var raw struct {
		Status struct {
			OperationState struct {
				Phase   string `json:"phase"`
				Message string `json:"message"`
				SyncResult struct {
					Revision string `json:"revision"`
					Resources []struct {
						Kind    string `json:"kind"`
						Name    string `json:"name"`
						Message string `json:"message"`
						Status  string `json:"status"`
					} `json:"resources"`
				} `json:"syncResult"`
			} `json:"operationState"`
		} `json:"status"`
	}

	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/applications/"+name+"/sync", payload, &raw); err != nil {
		return nil, fmt.Errorf("syncing application %q: %w", name, err)
	}

	result := &SyncResult{
		Phase:    raw.Status.OperationState.Phase,
		Message:  raw.Status.OperationState.Message,
		Revision: raw.Status.OperationState.SyncResult.Revision,
	}
	for _, r := range raw.Status.OperationState.SyncResult.Resources {
		result.Results = append(result.Results, ResourceResult{
			Kind:    r.Kind,
			Name:    r.Name,
			Message: r.Message,
			Status:  r.Status,
		})
	}
	return result, nil
}

// ─── Rollback ─────────────────────────────────────────────────────────────

func (c *httpClient) RollbackApplication(ctx context.Context, name string, revision int64) (*RollbackResult, error) {
	payload := map[string]interface{}{
		"id":    revision,
		"prune": true,
	}
	var raw struct {
		Status struct {
			OperationState struct {
				Phase   string `json:"phase"`
				Message string `json:"message"`
			} `json:"operationState"`
		} `json:"status"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/applications/"+name+"/rollback", payload, &raw); err != nil {
		return nil, fmt.Errorf("rolling back application %q to revision %d: %w", name, revision, err)
	}
	return &RollbackResult{
		Phase:   raw.Status.OperationState.Phase,
		Message: raw.Status.OperationState.Message,
	}, nil
}

func (c *httpClient) GetApplicationHistory(ctx context.Context, name string) ([]RevisionHistory, error) {
	var raw argoCDApp
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/applications/"+name, nil, &raw); err != nil {
		return nil, fmt.Errorf("getting application history %q: %w", name, err)
	}
	history := make([]RevisionHistory, 0, len(raw.Status.History))
	for _, h := range raw.Status.History {
		rh := RevisionHistory{
			ID:         h.ID,
			Revision:   h.Revision,
			DeployedAt: h.DeployedAt,
		}
		if h.InitiatedBy != nil {
			rh.DeployedBy = h.InitiatedBy.Username
		}
		rh.Source.RepoURL = h.Source.RepoURL
		rh.Source.Path = h.Source.Path
		rh.Source.TargetRevision = h.Source.TargetRevision
		history = append(history, rh)
	}
	return history, nil
}

// ─── Managed Resources ─────────────────────────────────────────────────────

func (c *httpClient) GetManagedResources(ctx context.Context, name string) ([]ManagedResource, error) {
	var raw struct {
		Items []struct {
			Kind                string `json:"kind"`
			Name                string `json:"name"`
			Namespace           string `json:"namespace"`
			Group               string `json:"group"`
			Version             string `json:"version"`
			Status              string `json:"status"`
			NormalizedLiveState string `json:"normalizedLiveState,omitempty"`
			PredictedLiveState  string `json:"predictedLiveState,omitempty"`
			Health              *struct {
				Status string `json:"status"`
			} `json:"health,omitempty"`
		} `json:"items"`
	}

	path := fmt.Sprintf("/api/v1/applications/%s/managed-resources", name)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, fmt.Errorf("getting managed resources for %q: %w", name, err)
	}

	resources := make([]ManagedResource, 0, len(raw.Items))
	for _, item := range raw.Items {
		mr := ManagedResource{
			Kind:                item.Kind,
			Name:                item.Name,
			Namespace:           item.Namespace,
			Group:               item.Group,
			Version:             item.Version,
			Status:              item.Status,
			NormalizedLiveState: item.NormalizedLiveState,
			PredictedLiveState:  item.PredictedLiveState,
		}
		if item.Health != nil {
			mr.Health = item.Health.Status
		}
		resources = append(resources, mr)
	}
	return resources, nil
}

func (c *httpClient) GetApplicationDiff(ctx context.Context, name string) (*DiffResult, error) {
	resources, err := c.GetManagedResources(ctx, name)
	if err != nil {
		return nil, err
	}

	diff := &DiffResult{AppName: name}
	for _, r := range resources {
		if r.NormalizedLiveState == "" && r.PredictedLiveState == "" {
			continue
		}
		rd := ResourceDiff{
			Kind:      r.Kind,
			Name:      r.Name,
			Namespace: r.Namespace,
			Live:      r.NormalizedLiveState,
			Target:    r.PredictedLiveState,
		}
		if r.NormalizedLiveState != r.PredictedLiveState {
			diff.Changed++
			rd.Diff = "~ changed"
		}
		diff.Resources = append(diff.Resources, rd)
	}
	return diff, nil
}

// ─── Auto-Sync ─────────────────────────────────────────────────────────────

func (c *httpClient) DisableAutoSync(ctx context.Context, name string) error {
	// Get current app spec, then patch out the syncPolicy.automated field
	var raw struct {
		Spec struct {
			SyncPolicy *struct {
				Automated interface{} `json:"automated,omitempty"`
			} `json:"syncPolicy,omitempty"`
		} `json:"spec"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/applications/"+name, nil, &raw); err != nil {
		return fmt.Errorf("getting app for disable-autosync: %w", err)
	}

	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"syncPolicy": nil,
		},
	}
	if err := c.doJSON(ctx, http.MethodPatch, "/api/v1/applications/"+name, patch, nil); err != nil {
		return fmt.Errorf("disabling auto-sync for %q: %w", name, err)
	}
	return nil
}

func (c *httpClient) EnableAutoSync(ctx context.Context, name string) error {
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"syncPolicy": map[string]interface{}{
				"automated": map[string]interface{}{
					"prune":    true,
					"selfHeal": true,
				},
			},
		},
	}
	if err := c.doJSON(ctx, http.MethodPatch, "/api/v1/applications/"+name, patch, nil); err != nil {
		return fmt.Errorf("enabling auto-sync for %q: %w", name, err)
	}
	return nil
}

// ─── Health ────────────────────────────────────────────────────────────────

func (c *httpClient) Ping(ctx context.Context) error {
	var raw struct {
		Version string `json:"Version"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/version", nil, &raw); err != nil {
		return fmt.Errorf("pinging argocd: %w", err)
	}
	return nil
}
