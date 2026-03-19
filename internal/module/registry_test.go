package module

import (
	"context"
	"errors"
	"testing"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// mockModule implements Module for testing.
type mockModule struct {
	name     string
	started  bool
	stopped  bool
	startErr error
	stopErr  error
}

func (m *mockModule) Name() string        { return m.name }
func (m *mockModule) Description() string { return "test module: " + m.name }
func (m *mockModule) Register(_ *telebot.Bot, _ *telebot.Group) {}
func (m *mockModule) Start(_ context.Context) error {
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	return nil
}
func (m *mockModule) Stop(_ context.Context) error {
	if m.stopErr != nil {
		return m.stopErr
	}
	m.stopped = true
	return nil
}
func (m *mockModule) Health() entity.HealthStatus { return entity.HealthStatusHealthy }
func (m *mockModule) Commands() []CommandInfo      { return nil }

func TestRegistry_Register(t *testing.T) {
	t.Parallel()

	r := NewRegistry(zap.NewNop())

	err := r.Register(&mockModule{name: "test"})
	require.NoError(t, err)

	names := r.Names()
	assert.Equal(t, []string{"test"}, names)
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	t.Parallel()

	r := NewRegistry(zap.NewNop())
	require.NoError(t, r.Register(&mockModule{name: "test"}))

	err := r.Register(&mockModule{name: "test"})
	assert.Error(t, err)
}

func TestRegistry_StartAll(t *testing.T) {
	t.Parallel()

	m1 := &mockModule{name: "mod1"}
	m2 := &mockModule{name: "mod2"}

	r := NewRegistry(zap.NewNop())
	require.NoError(t, r.Register(m1))
	require.NoError(t, r.Register(m2))

	err := r.StartAll(context.Background())
	require.NoError(t, err)

	assert.True(t, m1.started)
	assert.True(t, m2.started)
}

func TestRegistry_StartAll_FailureDoesNotBlock(t *testing.T) {
	t.Parallel()

	m1 := &mockModule{name: "mod1", startErr: errors.New("fail")}
	m2 := &mockModule{name: "mod2"}

	r := NewRegistry(zap.NewNop())
	require.NoError(t, r.Register(m1))
	require.NoError(t, r.Register(m2))

	err := r.StartAll(context.Background())
	require.NoError(t, err)

	assert.False(t, m1.started) // Failed but didn't block
	assert.True(t, m2.started)  // Still started
}

func TestRegistry_StopAll_ReverseOrder(t *testing.T) {
	t.Parallel()

	m1 := &mockModule{name: "mod1"}
	m2 := &mockModule{name: "mod2"}

	r := NewRegistry(zap.NewNop())
	require.NoError(t, r.Register(m1))
	require.NoError(t, r.Register(m2))

	r.StopAll(context.Background())

	assert.True(t, m1.stopped)
	assert.True(t, m2.stopped)
}

func TestRegistry_Get(t *testing.T) {
	t.Parallel()

	r := NewRegistry(zap.NewNop())
	require.NoError(t, r.Register(&mockModule{name: "test"}))

	m, ok := r.Get("test")
	assert.True(t, ok)
	assert.Equal(t, "test", m.Name())

	_, ok = r.Get("nonexistent")
	assert.False(t, ok)
}

func TestRegistry_HealthAll(t *testing.T) {
	t.Parallel()

	r := NewRegistry(zap.NewNop())
	require.NoError(t, r.Register(&mockModule{name: "test"}))

	statuses := r.HealthAll()
	assert.Equal(t, entity.HealthStatusHealthy, statuses["test"])
}
