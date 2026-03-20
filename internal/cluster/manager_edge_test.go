package cluster

import (
	"sync"
	"testing"

	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ─── UserContext edge cases ──────────────────────────────────────────────────

func TestUserContext_SetAndGet_SwitchCluster(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "prod", Default: true},
		{Name: "staging"},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	uc := NewUserContext(mgr)

	// Before setting: should fall back to default.
	cluster := uc.GetCluster(111)
	assert.Equal(t, "prod", cluster)

	// After setting: should return the set value.
	uc.SetCluster(111, "staging")
	assert.Equal(t, "staging", uc.GetCluster(111))
}

func TestUserContext_UnknownUser_FallsBackToDefault(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "alpha", Default: true},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	uc := NewUserContext(mgr)
	assert.Equal(t, "alpha", uc.GetCluster(999))
}

func TestUserContext_NoDefault_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	// Manager with no configs — no default.
	mgr := NewManager(nil, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	uc := NewUserContext(mgr)
	assert.Equal(t, "", uc.GetCluster(111))
}

func TestUserContext_ConcurrentAccess_NoRace(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "prod", Default: true},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	uc := NewUserContext(mgr)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			uc.SetCluster(int64(idx), "prod")
			_ = uc.GetCluster(int64(idx))
		}(i)
	}
	wg.Wait()
}

func TestUserContext_MultipleUsers_Independent(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "prod", Default: true},
		{Name: "staging"},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	uc := NewUserContext(mgr)

	uc.SetCluster(1, "prod")
	uc.SetCluster(2, "staging")

	assert.Equal(t, "prod", uc.GetCluster(1))
	assert.Equal(t, "staging", uc.GetCluster(2))
}

// ─── Manager edge cases ──────────────────────────────────────────────────────

func TestManager_NilConfigs_EmptyList(t *testing.T) {
	t.Parallel()

	mgr := NewManager(nil, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	assert.Empty(t, mgr.List())
}

func TestManager_Get_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "prod"},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	_, err := mgr.Get("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestManager_GetDefault_NoClusters_ReturnsError(t *testing.T) {
	t.Parallel()

	mgr := NewManager(nil, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	_, err := mgr.GetDefault()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no default")
}

func TestManager_GetDefault_FirstClusterIsDefault(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "alpha"},
		{Name: "beta"},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	info, err := mgr.GetDefault()
	require.NoError(t, err)
	assert.Equal(t, "alpha", info.Name)
	assert.True(t, info.IsDefault)
}

func TestManager_ExplicitDefault_Honored(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "alpha"},
		{Name: "beta", Default: true},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	info, err := mgr.GetDefault()
	require.NoError(t, err)
	assert.Equal(t, "beta", info.Name)
}

func TestManager_DisplayName_FallsBackToName(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "prod"},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	info, err := mgr.Get("prod")
	require.NoError(t, err)
	assert.Equal(t, "prod", info.DisplayName)
}

func TestManager_DisplayName_CustomSet(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "prod", DisplayName: "Production Cluster"},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	info, err := mgr.Get("prod")
	require.NoError(t, err)
	assert.Equal(t, "Production Cluster", info.DisplayName)
}

func TestManager_ClientSet_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()

	mgr := NewManager(nil, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	_, err := mgr.ClientSet("ghost")
	assert.Error(t, err)
}

func TestManager_MetricsClient_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()

	mgr := NewManager(nil, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	_, err := mgr.MetricsClient("ghost")
	assert.Error(t, err)
}

func TestManager_DynamicClient_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()

	mgr := NewManager(nil, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	_, err := mgr.DynamicClient("ghost")
	assert.Error(t, err)
}

func TestManager_Close_Idempotent(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{{Name: "a"}}, zap.NewNop())
	err := mgr.Close()
	assert.NoError(t, err)

	// Second close should not panic or block (channels can only be closed once,
	// so if it panics, the test framework catches it).
}

func TestManager_HealthCheck_NoClients_ReturnsUnknown(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "prod"},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	statuses := mgr.HealthCheck(t.Context())
	assert.Equal(t, entity.HealthStatusUnknown, statuses["prod"])
}

func TestManager_InitialStatus_IsUnknown(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "staging"},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	info, err := mgr.Get("staging")
	require.NoError(t, err)
	assert.Equal(t, entity.HealthStatusUnknown, info.Status)
}

func TestManager_List_ReturnsAllClusters(t *testing.T) {
	t.Parallel()

	mgr := NewManager([]config.ClusterConfig{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}, zap.NewNop())
	defer func() { _ = mgr.Close() }()

	list := mgr.List()
	assert.Len(t, list, 3)

	names := make([]string, len(list))
	for i, info := range list {
		names[i] = info.Name
	}
	assert.Contains(t, names, "a")
	assert.Contains(t, names, "b")
	assert.Contains(t, names, "c")
}
