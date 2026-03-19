// export_test.go exposes internal helpers for black-box testing from
// package helm_test. This file is ONLY compiled during `go test`.
package helm

import (
	"github.com/d9042n/telekube/internal/audit"
	"github.com/d9042n/telekube/internal/rbac"
	helmrelease "helm.sh/helm/v3/pkg/release"
	"go.uber.org/zap"
)

// NewModuleWithFactory is the test-only constructor that injects a custom
// ClientFactory, allowing tests to stub out Helm SDK calls without a real cluster.
func NewModuleWithFactory(
	clusters []ClusterClient,
	rbacEngine rbac.Engine,
	auditLogger audit.Logger,
	logger *zap.Logger,
	factory ClientFactory,
) *Module {
	return newModuleWithFactory(clusters, rbacEngine, auditLogger, logger, factory)
}

// TestListReleases exposes the private listReleases method for testing.
func (m *Module) TestListReleases(clusterName, namespace string) ([]*helmrelease.Release, error) {
	return m.listReleases(clusterName, namespace)
}

// TestGetReleaseDetail exposes the private getReleaseDetail method for testing.
func (m *Module) TestGetReleaseDetail(clusterName, namespace, releaseName string) (*helmrelease.Release, []*helmrelease.Release, error) {
	return m.getReleaseDetail(clusterName, namespace, releaseName)
}

// TestRollback exposes the private rollback method for testing.
func (m *Module) TestRollback(clusterName, namespace, releaseName string, version int) error {
	return m.rollback(clusterName, namespace, releaseName, version)
}
