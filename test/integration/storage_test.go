//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/d9042n/telekube/internal/storage/sqlite"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorage_SQLite_Ping(t *testing.T) {
	t.Parallel()

	store := newTestSQLiteStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := store.Ping(ctx)
	assert.NoError(t, err)
}

func TestStorage_SQLite_AuditLog(t *testing.T) {
	t.Parallel()

	store := newTestSQLiteStore(t)
	ctx := context.Background()

	auditStore := store.Audit()
	require.NotNil(t, auditStore)

	// Create an audit entry
	entry := &entity.AuditEntry{
		ID:         ulid.Make().String(),
		UserID:     123456,
		Username:   "testuser",
		Action:     "pods.list",
		Resource:   "pod/test-pod",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Status:     entity.AuditStatusSuccess,
		OccurredAt: time.Now().UTC(),
	}

	err := auditStore.Create(ctx, entry)
	require.NoError(t, err)

	// List audit entries
	filter := storage.AuditFilter{Page: 1, PageSize: 10}
	entries, total, err := auditStore.List(ctx, filter)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, 1)
	assert.NotEmpty(t, entries)
}

func TestStorage_SQLite_RBAC_UserRole(t *testing.T) {
	t.Parallel()

	store := newTestSQLiteStore(t)
	ctx := context.Background()

	rbacStore := store.RBAC()
	require.NotNil(t, rbacStore)

	const userID = int64(123456789)

	// Set a role
	err := rbacStore.SetUserRole(ctx, userID, "operator")
	require.NoError(t, err)

	// Get the role
	role, err := rbacStore.GetUserRole(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, "operator", role)

	// Update role
	err = rbacStore.SetUserRole(ctx, userID, "admin")
	require.NoError(t, err)

	role, err = rbacStore.GetUserRole(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, "admin", role)
}

// newTestSQLiteStore creates an in-memory SQLite store for testing.
func newTestSQLiteStore(t *testing.T) storage.Storage {
	t.Helper()
	store, err := sqlite.New(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}
