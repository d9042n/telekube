package leader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestDefaultConfig_EmptyNamespace(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig("")
	assert.Equal(t, "", cfg.LeaseNamespace)
	assert.Equal(t, "telekube-leader", cfg.LeaseName)
	assert.NotEmpty(t, cfg.Identity)
}

func TestDefaultConfig_Invariants(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig("test-ns")
	// LeaseDuration > RenewDeadline > RetryPeriod
	assert.Greater(t, cfg.LeaseDuration, cfg.RenewDeadline)
	assert.Greater(t, cfg.RenewDeadline, cfg.RetryPeriod)
	assert.Positive(t, cfg.RetryPeriod.Seconds())
}

func TestNewElector_SetsFields(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig("ns")
	called := false
	callbacks := Callbacks{
		OnStartedLeading: nil,
		OnStoppedLeading: func() { called = true },
	}

	e := NewElector(nil, cfg, callbacks, zap.NewNop())
	assert.NotNil(t, e)
	assert.Equal(t, cfg.LeaseName, e.cfg.LeaseName)
	assert.False(t, e.isLeader)

	// Test the callback was stored.
	e.callbacks.OnStoppedLeading()
	assert.True(t, called)
}

func TestElector_IsLeader_DefaultFalse(t *testing.T) {
	t.Parallel()

	e := &Elector{}
	assert.False(t, e.IsLeader())
}

func TestElector_Stop_NilCancel_NoOp(t *testing.T) {
	t.Parallel()

	e := &Elector{cancel: nil}
	// Should not panic.
	e.Stop()
}

func TestElector_Stop_Called_Idempotent(t *testing.T) {
	t.Parallel()

	cancelled := 0
	e := &Elector{cancel: func() { cancelled++ }}
	e.Stop()
	e.Stop()
	assert.Equal(t, 2, cancelled) // CancelFunc is safe to call multiple times.
}

func TestDefaultConfig_IdentityFallback(t *testing.T) {
	t.Parallel()

	// On any machine, identity should not be empty.
	cfg := DefaultConfig("prod")
	assert.NotEmpty(t, cfg.Identity)
}
