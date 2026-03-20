package cluster

import (
	"context"
	"testing"

	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewManager_NoConfigs(t *testing.T) {
	t.Parallel()

	m := NewManager(nil, zap.NewNop())
	clusters := m.List()
	assert.Empty(t, clusters)
}

func TestNewManager_WithConfigs(t *testing.T) {
	t.Parallel()

	configs := []config.ClusterConfig{
		{Name: "prod", DisplayName: "Production", Default: true},
		{Name: "staging", DisplayName: "Staging"},
	}

	m := NewManager(configs, zap.NewNop())
	defer func() { _ = m.Close() }()

	clusters := m.List()
	assert.Len(t, clusters, 2)
}

func TestManager_Get(t *testing.T) {
	t.Parallel()

	configs := []config.ClusterConfig{
		{Name: "prod", DisplayName: "Production", Default: true},
	}

	m := NewManager(configs, zap.NewNop())
	defer func() { _ = m.Close() }()

	info, err := m.Get("prod")
	require.NoError(t, err)
	assert.Equal(t, "prod", info.Name)
	assert.Equal(t, "Production", info.DisplayName)
	assert.True(t, info.IsDefault)
}

func TestManager_Get_NotFound(t *testing.T) {
	t.Parallel()

	m := NewManager(nil, zap.NewNop())
	defer func() { _ = m.Close() }()

	_, err := m.Get("nonexistent")
	assert.Error(t, err)
}

func TestManager_GetDefault(t *testing.T) {
	t.Parallel()

	configs := []config.ClusterConfig{
		{Name: "staging"},
		{Name: "prod", Default: true},
	}

	m := NewManager(configs, zap.NewNop())
	defer func() { _ = m.Close() }()

	info, err := m.GetDefault()
	require.NoError(t, err)
	assert.Equal(t, "prod", info.Name)
}

func TestManager_GetDefault_FallbackToFirst(t *testing.T) {
	t.Parallel()

	configs := []config.ClusterConfig{
		{Name: "first"},
		{Name: "second"},
	}

	m := NewManager(configs, zap.NewNop())
	defer func() { _ = m.Close() }()

	info, err := m.GetDefault()
	require.NoError(t, err)
	assert.Equal(t, "first", info.Name)
	assert.True(t, info.IsDefault)
}

func TestManager_GetDefault_NoConfigs(t *testing.T) {
	t.Parallel()

	m := NewManager(nil, zap.NewNop())
	defer func() { _ = m.Close() }()

	_, err := m.GetDefault()
	assert.Error(t, err)
}

func TestManager_DisplayName_Fallback(t *testing.T) {
	t.Parallel()

	configs := []config.ClusterConfig{
		{Name: "prod"}, // No DisplayName — should fall back to Name
	}

	m := NewManager(configs, zap.NewNop())
	defer func() { _ = m.Close() }()

	info, err := m.Get("prod")
	require.NoError(t, err)
	assert.Equal(t, "prod", info.DisplayName) // Falls back to Name
}

func TestManager_HealthCheck_UnknownWithoutClients(t *testing.T) {
	t.Parallel()

	configs := []config.ClusterConfig{
		{Name: "test", Kubeconfig: "/nonexistent"},
	}

	m := NewManager(configs, zap.NewNop())
	defer func() { _ = m.Close() }()

	statuses := m.HealthCheck(context.Background())
	assert.Equal(t, entity.HealthStatusUnknown, statuses["test"])
}

func TestManager_ClientSet_InvalidCluster(t *testing.T) {
	t.Parallel()

	m := NewManager(nil, zap.NewNop())
	defer func() { _ = m.Close() }()

	_, err := m.ClientSet("nonexistent")
	assert.Error(t, err)
}

func TestManager_MetricsClient_InvalidCluster(t *testing.T) {
	t.Parallel()

	m := NewManager(nil, zap.NewNop())
	defer func() { _ = m.Close() }()

	_, err := m.MetricsClient("nonexistent")
	assert.Error(t, err)
}

func TestManager_DynamicClient_InvalidCluster(t *testing.T) {
	t.Parallel()

	m := NewManager(nil, zap.NewNop())
	defer func() { _ = m.Close() }()

	_, err := m.DynamicClient("nonexistent")
	assert.Error(t, err)
}

func TestUserContext_Default(t *testing.T) {
	t.Parallel()

	configs := []config.ClusterConfig{
		{Name: "default-cluster", Default: true},
	}

	m := NewManager(configs, zap.NewNop())
	defer func() { _ = m.Close() }()

	uc := NewUserContext(m)

	// User has no explicit selection — should get default
	cluster := uc.GetCluster(100)
	assert.Equal(t, "default-cluster", cluster)
}

func TestUserContext_SetAndGet(t *testing.T) {
	t.Parallel()

	configs := []config.ClusterConfig{
		{Name: "prod", Default: true},
		{Name: "staging"},
	}

	m := NewManager(configs, zap.NewNop())
	defer func() { _ = m.Close() }()

	uc := NewUserContext(m)

	uc.SetCluster(100, "staging")
	assert.Equal(t, "staging", uc.GetCluster(100))

	// Other user still gets default
	assert.Equal(t, "prod", uc.GetCluster(200))
}
