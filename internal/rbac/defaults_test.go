package rbac

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhase2PermissionsExist(t *testing.T) {
	t.Parallel()

	// Verify Phase 2 permissions are defined
	assert.Equal(t, "kubernetes.metrics.view", PermKubernetesMetricsView)
	assert.Equal(t, "kubernetes.nodes.view", PermKubernetesNodesView)
	assert.Equal(t, "kubernetes.nodes.cordon", PermKubernetesNodesCordon)
	assert.Equal(t, "kubernetes.nodes.drain", PermKubernetesNodesDrain)
	assert.Equal(t, "kubernetes.quota.view", PermKubernetesQuotaView)
}

func TestDefaultRolesPhase2Permissions(t *testing.T) {
	t.Parallel()

	roles := defaultRoles()

	// Find viewer role
	var viewer, operator, admin []string
	for _, role := range roles {
		switch role.Name {
		case "viewer":
			viewer = role.Permissions
		case "operator":
			operator = role.Permissions
		case "admin":
			admin = role.Permissions
		}
	}

	// Viewer should have metrics view, nodes view, quota view
	assert.Contains(t, viewer, PermKubernetesMetricsView)
	assert.Contains(t, viewer, PermKubernetesNodesView)
	assert.Contains(t, viewer, PermKubernetesQuotaView)

	// Viewer should NOT have cordon, drain, or scale
	assert.NotContains(t, viewer, PermKubernetesNodesCordon)
	assert.NotContains(t, viewer, PermKubernetesNodesDrain)
	assert.NotContains(t, viewer, PermKubernetesDeploymentsScale)

	// Operator should have metrics, nodes, quota + scale
	assert.Contains(t, operator, PermKubernetesMetricsView)
	assert.Contains(t, operator, PermKubernetesNodesView)
	assert.Contains(t, operator, PermKubernetesQuotaView)
	assert.Contains(t, operator, PermKubernetesDeploymentsScale)

	// Operator should NOT have cordon, drain
	assert.NotContains(t, operator, PermKubernetesNodesCordon)
	assert.NotContains(t, operator, PermKubernetesNodesDrain)

	// Admin should have ALL Phase 2 permissions
	assert.Contains(t, admin, PermKubernetesMetricsView)
	assert.Contains(t, admin, PermKubernetesNodesView)
	assert.Contains(t, admin, PermKubernetesQuotaView)
	assert.Contains(t, admin, PermKubernetesNodesCordon)
	assert.Contains(t, admin, PermKubernetesNodesDrain)
	assert.Contains(t, admin, PermKubernetesDeploymentsScale)
}
