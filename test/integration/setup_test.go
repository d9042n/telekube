//go:build integration

// Package integration provides integration tests for Telekube.
// These tests require a running Kubernetes cluster (kind) and database services.
//
// Run with: go test ./test/integration/... -v -tags=integration
package integration

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfig holds the integration test configuration.
type TestConfig struct {
	PgDSN       string
	RedisAddr   string
	KubeContext string
}

// loadTestConfig loads test configuration from environment variables.
func loadTestConfig(t *testing.T) TestConfig {
	t.Helper()
	cfg := TestConfig{
		PgDSN:       getEnv("TEST_PG_DSN", "postgres://test:test@localhost:5433/telekube_test?sslmode=disable"),
		RedisAddr:   getEnv("TEST_REDIS_ADDR", "localhost:6380"),
		KubeContext: getEnv("TEST_KUBE_CONTEXT", "kind-telekube-test"),
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// requireEnv fails the test if an environment variable is not set.
func requireEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	require.NotEmpty(t, v, "required environment variable %s is not set", key)
	return v
}
