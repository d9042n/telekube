// Package leader provides Kubernetes Lease-based leader election for HA.
package leader

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// Callbacks defines functions called on leader state change.
type Callbacks struct {
	OnStartedLeading func(ctx context.Context)
	OnStoppedLeading func()
	OnNewLeader      func(identity string)
}

// Config holds leader election configuration.
type Config struct {
	LeaseName      string
	LeaseNamespace string
	Identity       string
	LeaseDuration  time.Duration
	RenewDeadline  time.Duration
	RetryPeriod    time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(namespace string) Config {
	identity, _ := os.Hostname()
	if identity == "" {
		identity = fmt.Sprintf("telekube-%d", os.Getpid())
	}

	return Config{
		LeaseName:      "telekube-leader",
		LeaseNamespace: namespace,
		Identity:       identity,
		LeaseDuration:  15 * time.Second,
		RenewDeadline:  10 * time.Second,
		RetryPeriod:    2 * time.Second,
	}
}

// Elector manages leader election.
type Elector struct {
	cfg       Config
	client    kubernetes.Interface
	callbacks Callbacks
	logger    *zap.Logger
	cancel    context.CancelFunc
	isLeader  bool
}

// NewElector creates a new leader elector.
func NewElector(client kubernetes.Interface, cfg Config, callbacks Callbacks, logger *zap.Logger) *Elector {
	return &Elector{
		cfg:       cfg,
		client:    client,
		callbacks: callbacks,
		logger:    logger,
	}
}

// Start begins participating in leader election.
func (e *Elector) Start(ctx context.Context) {
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      e.cfg.LeaseName,
			Namespace: e.cfg.LeaseNamespace,
		},
		Client: e.client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: e.cfg.Identity,
		},
	}

	leCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	go func() {
		leaderelection.RunOrDie(leCtx, leaderelection.LeaderElectionConfig{
			Lock:            lock,
			LeaseDuration:   e.cfg.LeaseDuration,
			RenewDeadline:   e.cfg.RenewDeadline,
			RetryPeriod:     e.cfg.RetryPeriod,
			ReleaseOnCancel: true,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(ctx context.Context) {
					e.isLeader = true
					e.logger.Info("started leading",
						zap.String("identity", e.cfg.Identity),
					)
					if e.callbacks.OnStartedLeading != nil {
						e.callbacks.OnStartedLeading(ctx)
					}
				},
				OnStoppedLeading: func() {
					e.isLeader = false
					e.logger.Info("stopped leading",
						zap.String("identity", e.cfg.Identity),
					)
					if e.callbacks.OnStoppedLeading != nil {
						e.callbacks.OnStoppedLeading()
					}
				},
				OnNewLeader: func(identity string) {
					if identity == e.cfg.Identity {
						return
					}
					e.logger.Info("new leader elected",
						zap.String("leader", identity),
						zap.String("identity", e.cfg.Identity),
					)
					if e.callbacks.OnNewLeader != nil {
						e.callbacks.OnNewLeader(identity)
					}
				},
			},
		})
	}()

	e.logger.Info("leader election started",
		zap.String("identity", e.cfg.Identity),
		zap.String("lease", e.cfg.LeaseName),
		zap.String("namespace", e.cfg.LeaseNamespace),
	)
}

// Stop stops participating in leader election.
func (e *Elector) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

// IsLeader returns whether this instance is the leader.
func (e *Elector) IsLeader() bool {
	return e.isLeader
}
