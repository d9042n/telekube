package argocd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/d9042n/telekube/pkg/argocd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestServer(handler http.HandlerFunc) (*httptest.Server, argocd.Client) {
	srv := httptest.NewServer(handler)
	client := argocd.NewClient(argocd.ClientConfig{
		BaseURL: srv.URL,
		Auth:    argocd.NewTokenAuth("test-token"),
		Timeout: 5 * time.Second,
		Logger:  zap.NewNop(),
	})
	return srv, client
}

func TestListApplications(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		expectLen  int
		expectErr  bool
	}{
		{
			name: "returns applications",
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/api/v1/applications", r.URL.Path)
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				json.NewEncoder(w).Encode(map[string]interface{}{
					"items": []map[string]interface{}{
						{
							"metadata": map[string]interface{}{"name": "my-app", "namespace": "argocd"},
							"spec": map[string]interface{}{
								"project": "default",
								"source": map[string]interface{}{
									"repoURL": "https://github.com/org/repo",
									"path":    "k8s",
									"targetRevision": "HEAD",
								},
							},
							"status": map[string]interface{}{
								"sync":   map[string]interface{}{"status": "Synced", "revision": "abc123"},
								"health": map[string]interface{}{"status": "Healthy"},
							},
						},
					},
				})
			},
			expectLen: 1,
			expectErr: false,
		},
		{
			name: "empty list",
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]interface{}{"items": []interface{}{}})
			},
			expectLen: 0,
			expectErr: false,
		},
		{
			name: "server error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("internal error"))
			},
			expectLen: 0,
			expectErr: true,
		},
		{
			name: "unauthorized",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("unauthorized"))
			},
			expectLen: 0,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, client := newTestServer(tt.handler)
			defer srv.Close()

			apps, err := client.ListApplications(context.Background(), argocd.ListOpts{})
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, apps, tt.expectLen)
		})
	}
}

func TestGetApplication(t *testing.T) {
	t.Parallel()

	srv, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/applications/my-app", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"metadata": map[string]interface{}{"name": "my-app", "namespace": "argocd"},
			"spec": map[string]interface{}{
				"project": "default",
				"source": map[string]interface{}{
					"repoURL": "https://github.com/org/repo",
					"path":    "k8s",
					"targetRevision": "HEAD",
				},
			},
			"status": map[string]interface{}{
				"sync":   map[string]interface{}{"status": "Synced", "revision": "abc123"},
				"health": map[string]interface{}{"status": "Healthy"},
			},
		})
	})
	defer srv.Close()

	app, err := client.GetApplication(context.Background(), "my-app")
	require.NoError(t, err)
	assert.Equal(t, "my-app", app.Name)
	assert.Equal(t, "Synced", app.SyncStatus)
	assert.Equal(t, "Healthy", app.HealthStatus)
	assert.Equal(t, "abc123", app.CurrentRev)
}

func TestSyncApplication(t *testing.T) {
	t.Parallel()

	srv, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "/sync")

		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		assert.Equal(t, false, payload["prune"])

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": map[string]interface{}{
				"operationState": map[string]interface{}{
					"phase":   "Succeeded",
					"message": "successfully synced",
					"syncResult": map[string]interface{}{
						"revision": "def456",
						"resources": []interface{}{},
					},
				},
			},
		})
	})
	defer srv.Close()

	result, err := client.SyncApplication(context.Background(), "my-app", argocd.SyncOpts{Prune: false})
	require.NoError(t, err)
	assert.Equal(t, "Succeeded", result.Phase)
	assert.Equal(t, "def456", result.Revision)
}

func TestPing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		handler   http.HandlerFunc
		expectErr bool
	}{
		{
			name: "healthy",
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/version", r.URL.Path)
				json.NewEncoder(w).Encode(map[string]interface{}{"Version": "2.8.0"})
			},
			expectErr: false,
		},
		{
			name: "unhealthy",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, client := newTestServer(tt.handler)
			defer srv.Close()

			err := client.Ping(context.Background())
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTokenAuth(t *testing.T) {
	t.Parallel()

	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	auth := argocd.NewTokenAuth("my-secret-token")
	require.NoError(t, auth.Apply(req))
	assert.Equal(t, "Bearer my-secret-token", req.Header.Get("Authorization"))
}
