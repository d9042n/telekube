package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChecker(t *testing.T) {
	c := NewChecker()
	require.NotNil(t, c)
	assert.Empty(t, c.checks)
}

func TestChecker_Register(t *testing.T) {
	c := NewChecker()
	c.Register("db", func(ctx context.Context) error { return nil })
	c.Register("cache", func(ctx context.Context) error { return nil })

	assert.Len(t, c.checks, 2)
}

func TestChecker_CheckAll_AllHealthy(t *testing.T) {
	c := NewChecker()
	c.Register("db", func(ctx context.Context) error { return nil })
	c.Register("cache", func(ctx context.Context) error { return nil })

	results := c.CheckAll(context.Background())

	assert.Len(t, results, 2)
	assert.Equal(t, "healthy", results["db"])
	assert.Equal(t, "healthy", results["cache"])
}

func TestChecker_CheckAll_OneUnhealthy(t *testing.T) {
	c := NewChecker()
	c.Register("db", func(ctx context.Context) error { return nil })
	c.Register("cache", func(ctx context.Context) error { return errors.New("connection refused") })

	results := c.CheckAll(context.Background())

	assert.Equal(t, "healthy", results["db"])
	assert.Contains(t, results["cache"], "unhealthy")
	assert.Contains(t, results["cache"], "connection refused")
}

func TestChecker_CheckAll_NoChecks(t *testing.T) {
	c := NewChecker()
	results := c.CheckAll(context.Background())
	assert.Empty(t, results)
}

func TestChecker_IsReady_AllHealthy(t *testing.T) {
	c := NewChecker()
	c.Register("db", func(ctx context.Context) error { return nil })

	assert.True(t, c.IsReady(context.Background()))
}

func TestChecker_IsReady_OneUnhealthy(t *testing.T) {
	c := NewChecker()
	c.Register("db", func(ctx context.Context) error { return nil })
	c.Register("cache", func(ctx context.Context) error { return errors.New("fail") })

	assert.False(t, c.IsReady(context.Background()))
}

func TestChecker_IsReady_NoChecks(t *testing.T) {
	c := NewChecker()
	// With no checks, IsReady should return true (vacuously true)
	assert.True(t, c.IsReady(context.Background()))
}

// --- HTTP handler tests ---

func TestHealthzHandler_Returns200(t *testing.T) {
	c := NewChecker()
	s := NewServer(0, c)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	s.handleHealthz(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var body map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
}

func TestReadyzHandler_AllHealthy(t *testing.T) {
	c := NewChecker()
	c.Register("db", func(ctx context.Context) error { return nil })
	s := NewServer(0, c)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	s.handleReadyz(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "ready", body["status"])

	checks, ok := body["checks"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "healthy", checks["db"])
}

func TestReadyzHandler_OneUnhealthy(t *testing.T) {
	c := NewChecker()
	c.Register("db", func(ctx context.Context) error { return nil })
	c.Register("cache", func(ctx context.Context) error { return errors.New("timeout") })
	s := NewServer(0, c)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	s.handleReadyz(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "not_ready", body["status"])

	checks, ok := body["checks"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, checks["cache"], "unhealthy")
}

func TestReadyzHandler_NoChecks(t *testing.T) {
	c := NewChecker()
	s := NewServer(0, c)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	s.handleReadyz(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, "ready", body["status"])
}

func TestNewServer(t *testing.T) {
	c := NewChecker()
	s := NewServer(8080, c)
	require.NotNil(t, s)
	require.NotNil(t, s.server)
	assert.Equal(t, ":8080", s.server.Addr)
}

func TestHealthzHandler_ContentType(t *testing.T) {
	c := NewChecker()
	s := NewServer(0, c)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	s.handleHealthz(rr, req)

	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
}

func TestReadyzHandler_ContentType(t *testing.T) {
	c := NewChecker()
	s := NewServer(0, c)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	s.handleReadyz(rr, req)

	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
}
