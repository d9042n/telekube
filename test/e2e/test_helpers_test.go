//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	e2e "github.com/d9042n/telekube/test/e2e"
)

// newFakeServer creates a FakeTelegramServer and registers cleanup.
func newFakeServer(t *testing.T) *e2e.FakeTelegramServer {
	t.Helper()
	srv := e2e.NewFakeTelegramServer()
	t.Cleanup(srv.Close)
	return srv
}

// newSmokeHarness creates a Harness without k3s (fast, no Docker for cluster).
func newSmokeHarness(t *testing.T, adminID int64) *e2e.Harness {
	t.Helper()
	return e2e.NewHarness(t,
		e2e.WithoutK3s(),
		e2e.WithAdminIDs(adminID),
	)
}

// get performs an HTTP GET to url and returns the response.
func get(t *testing.T, url string) (*http.Response, error) {
	t.Helper()
	//nolint:noctx // test helper, no timeout needed beyond test itself
	return http.Get(url) //nolint:gosec // test-only, URL is local
}

// mustGetBody performs a GET and returns the response body as a string.
func mustGetBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := get(t, url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body from %s: %v", url, err)
	}
	return string(body)
}

// mustPostJSON performs an HTTP POST with a JSON payload and returns the body.
func mustPostJSON(t *testing.T, url string, payload interface{}) string {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshaling payload: %v", err)
	}
	//nolint:noctx // test helper
	resp, err := http.Post(url, "application/json", bytes.NewReader(data)) //nolint:gosec
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	return string(body)
}
