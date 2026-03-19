package helm

import (
	"fmt"
	"time"

	helmaction "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	helmrelease "helm.sh/helm/v3/pkg/release"

	k8s "k8s.io/client-go/rest"
)

// ReleaseClient abstracts Helm SDK operations so they can be swapped in tests.
// All methods are cluster-scoped: the factory already received the cluster REST
// config and namespace, so callers only pass release names / revision numbers.
type ReleaseClient interface {
	// ListReleases returns all releases visible within the configured namespace.
	ListReleases() ([]*helmrelease.Release, error)
	// GetRelease returns a single release by name.
	GetRelease(name string) (*helmrelease.Release, error)
	// GetHistory returns up to 10 previous revisions.
	GetHistory(name string) ([]*helmrelease.Release, error)
	// Rollback rolls back a release to the given revision number.
	Rollback(name string, version int) error
}

// ClientFactory is responsible for building a ReleaseClient for a specific
// cluster + namespace.  The production implementation uses the Helm SDK;
// tests inject a stub factory.
type ClientFactory func(kubeconfig *k8s.Config, namespace string) (ReleaseClient, error)

// DefaultClientFactory is the production factory that constructs a real Helm
// action-based client.
func DefaultClientFactory(kubeconfig *k8s.Config, namespace string) (ReleaseClient, error) {
	settings := cli.New()
	_ = settings // helm env vars are still respected via the settings object

	cfg := new(helmaction.Configuration)
	getter := &restConfigGetter{cfg: kubeconfig, namespace: namespace}
	if err := cfg.Init(getter, namespace, "secret", func(format string, v ...interface{}) {
		// Helm debug logging is discarded in production; the module logger is used instead.
		_ = fmt.Sprintf(format, v...)
	}); err != nil {
		return nil, fmt.Errorf("init helm action config: %w", err)
	}
	return &sdkClient{cfg: cfg}, nil
}

// sdkClient is the production implementation of ReleaseClient backed by
// helm.sh/helm/v3/pkg/action.
type sdkClient struct {
	cfg *helmaction.Configuration
}

func (c *sdkClient) ListReleases() ([]*helmrelease.Release, error) {
	la := helmaction.NewList(c.cfg)
	la.All = true
	return la.Run()
}

func (c *sdkClient) GetRelease(name string) (*helmrelease.Release, error) {
	return helmaction.NewGet(c.cfg).Run(name)
}

func (c *sdkClient) GetHistory(name string) ([]*helmrelease.Release, error) {
	ha := helmaction.NewHistory(c.cfg)
	ha.Max = 10
	return ha.Run(name)
}

func (c *sdkClient) Rollback(name string, version int) error {
	ra := helmaction.NewRollback(c.cfg)
	ra.Version = version
	ra.Wait = true
	ra.Timeout = 5 * time.Minute
	return ra.Run(name)
}
