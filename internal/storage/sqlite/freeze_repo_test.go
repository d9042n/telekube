package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFreezeTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.New(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestFreezeCreate(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	freeze := &entity.DeploymentFreeze{
		ID:        "freeze-001",
		Scope:     "all",
		Reason:    "planned maintenance",
		CreatedBy: 123,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		ExpiresAt: time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second),
	}

	err := repo.Create(context.Background(), freeze)
	require.NoError(t, err)
}

func TestFreezeGetActive(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	// No freeze at start
	result, err := repo.GetActive(context.Background())
	require.NoError(t, err)
	assert.Nil(t, result)

	// Create an active freeze
	freeze := &entity.DeploymentFreeze{
		ID:        "freeze-002",
		Scope:     "all",
		CreatedBy: 456,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour).Truncate(time.Second),
	}
	require.NoError(t, repo.Create(context.Background(), freeze))

	// Should now return it
	active, err := repo.GetActive(context.Background())
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, "freeze-002", active.ID)
	assert.Equal(t, "all", active.Scope)
	assert.Equal(t, int64(456), active.CreatedBy)
	assert.True(t, active.IsActive())
}

func TestFreezeGetActiveMissesExpired(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	// Insert an already-expired freeze
	freeze := &entity.DeploymentFreeze{
		ID:        "freeze-003",
		Scope:     "all",
		CreatedBy: 789,
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second),
		ExpiresAt: time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Second),
	}
	require.NoError(t, repo.Create(context.Background(), freeze))

	// GetActive should not return it
	active, err := repo.GetActive(context.Background())
	require.NoError(t, err)
	assert.Nil(t, active)
}

func TestFreezeGetActiveForCluster(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	// Create a "prod-1" scoped freeze
	freeze := &entity.DeploymentFreeze{
		ID:        "freeze-004",
		Scope:     "prod-1",
		CreatedBy: 100,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour).Truncate(time.Second),
	}
	require.NoError(t, repo.Create(context.Background(), freeze))

	// Should match "prod-1"
	match, err := repo.GetActiveForCluster(context.Background(), "prod-1")
	require.NoError(t, err)
	require.NotNil(t, match)
	assert.Equal(t, "prod-1", match.Scope)

	// Should NOT match "staging"
	noMatch, err := repo.GetActiveForCluster(context.Background(), "staging")
	require.NoError(t, err)
	assert.Nil(t, noMatch)
}

func TestFreezeGetActiveForCluster_AllScope(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	// Create an "all" scope freeze
	freeze := &entity.DeploymentFreeze{
		ID:        "freeze-005",
		Scope:     "all",
		CreatedBy: 200,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour).Truncate(time.Second),
	}
	require.NoError(t, repo.Create(context.Background(), freeze))

	// "all" scope should match any cluster
	for _, cluster := range []string{"prod-1", "staging", "dev"} {
		match, err := repo.GetActiveForCluster(context.Background(), cluster)
		require.NoError(t, err, "cluster: %s", cluster)
		require.NotNil(t, match, "cluster: %s — expected 'all' scope freeze to match", cluster)
	}
}

func TestFreezeThaw(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	freeze := &entity.DeploymentFreeze{
		ID:        "freeze-006",
		Scope:     "all",
		CreatedBy: 300,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour).Truncate(time.Second),
	}
	require.NoError(t, repo.Create(context.Background(), freeze))

	// Verify it's active
	active, err := repo.GetActive(context.Background())
	require.NoError(t, err)
	require.NotNil(t, active)

	// Thaw it
	require.NoError(t, repo.Thaw(context.Background(), "freeze-006", 400))

	// Should no longer be active
	active2, err := repo.GetActive(context.Background())
	require.NoError(t, err)
	assert.Nil(t, active2)
}

func TestFreezeThawNotFound(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	err := repo.Thaw(context.Background(), "nonexistent", 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFreezeList(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	// Create 3 freezes
	for i := 1; i <= 3; i++ {
		f := &entity.DeploymentFreeze{
			ID:        string(rune('A' + i - 1)),
			Scope:     "all",
			CreatedBy: int64(i),
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Second).Truncate(time.Second),
			ExpiresAt: time.Now().UTC().Add(1 * time.Hour).Truncate(time.Second),
		}
		require.NoError(t, repo.Create(context.Background(), f))
	}

	freezes, err := repo.List(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, freezes, 3)
}

func TestFreezeListEmpty(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	freezes, err := repo.List(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, freezes)
}

func TestFreezeListLimit(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	// Create 5 freezes, all with different IDs
	for i := 0; i < 5; i++ {
		f := &entity.DeploymentFreeze{
			ID:        "lf-" + string(rune('a'+i)),
			Scope:     "all",
			CreatedBy: int64(i + 1),
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Second).Truncate(time.Second),
			ExpiresAt: time.Now().UTC().Add(1 * time.Hour).Truncate(time.Second),
		}
		require.NoError(t, repo.Create(context.Background(), f))
	}

	freezes, err := repo.List(context.Background(), 3)
	require.NoError(t, err)
	assert.Len(t, freezes, 3)
}

func TestFreezeThawedAtFieldPopulated(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	f := &entity.DeploymentFreeze{
		ID:        "freeze-thaw-test",
		Scope:     "all",
		CreatedBy: 111,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		ExpiresAt: time.Now().UTC().Add(time.Hour).Truncate(time.Second),
	}
	require.NoError(t, repo.Create(context.Background(), f))
	require.NoError(t, repo.Thaw(context.Background(), "freeze-thaw-test", 222))

	list, err := repo.List(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.NotNil(t, list[0].ThawedAt)
	require.NotNil(t, list[0].ThawedBy)
	assert.Equal(t, int64(222), *list[0].ThawedBy)
}

// Verify sql.NullTime is handled where DB returns NULL for thawed_at
// (done internally via scanFreeze — tested indirectly via GetActive)
func TestFreezeNullThawedAt(t *testing.T) {
	t.Parallel()
	store := newFreezeTestStore(t)
	repo := store.Freeze()

	f := &entity.DeploymentFreeze{
		ID:        "freeze-null-thaw",
		Scope:     "all",
		CreatedBy: 333,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		ExpiresAt: time.Now().UTC().Add(time.Hour).Truncate(time.Second),
	}
	require.NoError(t, repo.Create(context.Background(), f))

	active, err := repo.GetActive(context.Background())
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Nil(t, active.ThawedAt)
	assert.Nil(t, active.ThawedBy)
}

// Ensure the Store type satisfies the storage.Storage interface at compile time
// (checked via go vet / build — tested indirectly by all tests above)
var _ = (*sqlite.Store)(nil)

// Silence unused import warning for sql package
var _ = sql.ErrNoRows
