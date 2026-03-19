package leader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig("telekube")

	assert.Equal(t, "telekube-leader", cfg.LeaseName)
	assert.Equal(t, "telekube", cfg.LeaseNamespace)
	assert.NotEmpty(t, cfg.Identity)
	assert.Greater(t, cfg.LeaseDuration.Seconds(), float64(0))
	assert.Greater(t, cfg.RenewDeadline.Seconds(), float64(0))
	assert.Greater(t, cfg.RetryPeriod.Seconds(), float64(0))
	assert.Greater(t, cfg.LeaseDuration, cfg.RenewDeadline, "lease duration must be greater than renew deadline")
	assert.Greater(t, cfg.RenewDeadline, cfg.RetryPeriod, "renew deadline must be greater than retry period")
}

func TestElectorIsLeader(t *testing.T) {
	t.Parallel()

	e := &Elector{isLeader: false}
	assert.False(t, e.IsLeader())

	e.isLeader = true
	assert.True(t, e.IsLeader())
}
