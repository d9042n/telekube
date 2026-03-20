package rbacmod

import (
	"context"
	"testing"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Module interface tests ---

func TestModule_Name(t *testing.T) {
	m := &Module{healthy: true, logger: zap.NewNop()}
	assert.Equal(t, "rbac", m.Name())
}

func TestModule_Description(t *testing.T) {
	m := &Module{healthy: true, logger: zap.NewNop()}
	assert.Equal(t, "RBAC role management via Telegram", m.Description())
}

func TestModule_Health(t *testing.T) {
	m := &Module{healthy: true, logger: zap.NewNop()}
	assert.Equal(t, entity.HealthStatusHealthy, m.Health())

	m.healthy = false
	assert.Equal(t, entity.HealthStatusUnhealthy, m.Health())
}

func TestModule_Commands(t *testing.T) {
	m := &Module{healthy: true, logger: zap.NewNop()}
	cmds := m.Commands()
	require.Len(t, cmds, 1)
	assert.Equal(t, "/rbac", cmds[0].Command)
	assert.Equal(t, rbac.PermAdminRBACManage, cmds[0].Permission)
	assert.Equal(t, "all", cmds[0].ChatType)
}

func TestModule_StartStop(t *testing.T) {
	m := &Module{logger: zap.NewNop()}
	require.NoError(t, m.Start(context.TODO()))
	require.NoError(t, m.Stop(context.TODO()))
}

func TestNewModule(t *testing.T) {
	m := NewModule(nil, nil, nil, zap.NewNop())
	require.NotNil(t, m)
	assert.True(t, m.healthy)
	assert.Equal(t, entity.HealthStatusHealthy, m.Health())
}
