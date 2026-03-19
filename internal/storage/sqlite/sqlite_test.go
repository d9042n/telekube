package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestUserRepo_Upsert_and_Get(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	user := &entity.User{
		TelegramID:  12345,
		Username:    "testuser",
		DisplayName: "Test User",
		Role:        entity.RoleViewer,
		IsActive:    true,
	}

	err := store.Users().Upsert(context.Background(), user)
	require.NoError(t, err)

	got, err := store.Users().GetByTelegramID(context.Background(), 12345)
	require.NoError(t, err)
	assert.Equal(t, user.TelegramID, got.TelegramID)
	assert.Equal(t, "testuser", got.Username)
	assert.Equal(t, "Test User", got.DisplayName)
	assert.Equal(t, entity.RoleViewer, got.Role)
	assert.True(t, got.IsActive)
}

func TestUserRepo_Upsert_UpdatesOnConflict(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	user := &entity.User{
		TelegramID:  12345,
		Username:    "old_name",
		DisplayName: "Old",
		Role:        entity.RoleViewer,
		IsActive:    true,
	}
	require.NoError(t, store.Users().Upsert(context.Background(), user))

	user.Username = "new_name"
	user.DisplayName = "New"
	require.NoError(t, store.Users().Upsert(context.Background(), user))

	got, err := store.Users().GetByTelegramID(context.Background(), 12345)
	require.NoError(t, err)
	assert.Equal(t, "new_name", got.Username)
	assert.Equal(t, "New", got.DisplayName)
}

func TestUserRepo_GetByTelegramID_NotFound(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	_, err := store.Users().GetByTelegramID(context.Background(), 99999)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestUserRepo_List(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	users := []*entity.User{
		{TelegramID: 1, Username: "a", Role: entity.RoleViewer, IsActive: true},
		{TelegramID: 2, Username: "b", Role: entity.RoleOperator, IsActive: true},
		{TelegramID: 3, Username: "c", Role: entity.RoleAdmin, IsActive: true},
	}
	for _, u := range users {
		require.NoError(t, store.Users().Upsert(context.Background(), u))
	}

	list, err := store.Users().List(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestAuditRepo_Create_and_List(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	entry := &entity.AuditEntry{
		ID:         "test-001",
		UserID:     12345,
		Username:   "testuser",
		Action:     "pod.restart",
		Resource:   "pod/my-pod",
		Cluster:    "prod",
		Namespace:  "default",
		ChatID:     100,
		ChatType:   "private",
		Status:     entity.AuditStatusSuccess,
		OccurredAt: time.Now().UTC(),
	}

	err := store.Audit().Create(context.Background(), entry)
	require.NoError(t, err)

	filter := storage.AuditFilter{
		Page:     1,
		PageSize: 10,
	}
	entries, total, err := store.Audit().List(context.Background(), filter)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, entries, 1)
	assert.Equal(t, "test-001", entries[0].ID)
	assert.Equal(t, "pod.restart", entries[0].Action)
}

func TestAuditRepo_List_WithFilters(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	now := time.Now().UTC()
	entries := []*entity.AuditEntry{
		{ID: "1", UserID: 100, Username: "alice", Action: "pod.list", Cluster: "prod", Status: "success", OccurredAt: now},
		{ID: "2", UserID: 200, Username: "bob", Action: "pod.restart", Cluster: "staging", Status: "success", OccurredAt: now},
		{ID: "3", UserID: 100, Username: "alice", Action: "pod.restart", Cluster: "prod", Status: "denied", OccurredAt: now},
	}
	for _, e := range entries {
		require.NoError(t, store.Audit().Create(context.Background(), e))
	}

	// Filter by user
	userID := int64(100)
	result, total, err := store.Audit().List(context.Background(), storage.AuditFilter{
		UserID:   &userID,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, result, 2)

	// Filter by cluster
	cluster := "prod"
	result, total, err = store.Audit().List(context.Background(), storage.AuditFilter{
		Cluster:  &cluster,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, total)

	// Filter by action
	action := "pod.restart"
	result, total, err = store.Audit().List(context.Background(), storage.AuditFilter{
		Action:   &action,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, result, 2)
}

func TestAuditRepo_List_Pagination(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	for i := 0; i < 25; i++ {
		require.NoError(t, store.Audit().Create(context.Background(), &entity.AuditEntry{
			ID:         fmt.Sprintf("entry-%03d", i),
			UserID:     100,
			Username:   "user",
			Action:     "test",
			Status:     "success",
			OccurredAt: time.Now().UTC(),
		}))
	}

	// Page 1
	result, total, err := store.Audit().List(context.Background(), storage.AuditFilter{
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 25, total)
	assert.Len(t, result, 10)

	// Page 3 (last page)
	result, total, err = store.Audit().List(context.Background(), storage.AuditFilter{
		Page:     3,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 25, total)
	assert.Len(t, result, 5)
}

func TestRBACRepo_GetSetRole(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// First create the user
	require.NoError(t, store.Users().Upsert(context.Background(), &entity.User{
		TelegramID: 100,
		Username:   "test",
		Role:       entity.RoleViewer,
		IsActive:   true,
	}))

	role, err := store.RBAC().GetUserRole(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, entity.RoleViewer, role)

	err = store.RBAC().SetUserRole(context.Background(), 100, entity.RoleAdmin)
	require.NoError(t, err)

	role, err = store.RBAC().GetUserRole(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, entity.RoleAdmin, role)
}

func TestRBACRepo_GetUserRole_NotFound(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	_, err := store.RBAC().GetUserRole(context.Background(), 99999)
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestStore_Ping(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	err := store.Ping(context.Background())
	assert.NoError(t, err)
}
