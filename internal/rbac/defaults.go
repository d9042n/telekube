package rbac

import (
	"time"

	"github.com/d9042n/telekube/internal/entity"
)

// Permission constants following <module>.<resource>.<action> format.
const (
	// Kubernetes permissions — pods
	PermKubernetesPodsList    = "kubernetes.pods.list"
	PermKubernetesPodsGet     = "kubernetes.pods.get"
	PermKubernetesPodsLogs    = "kubernetes.pods.logs"
	PermKubernetesPodsEvents  = "kubernetes.pods.events"
	PermKubernetesPodsRestart = "kubernetes.pods.restart"
	PermKubernetesPodsDelete  = "kubernetes.pods.delete"

	// Kubernetes permissions — deployments
	PermKubernetesDeploymentsList  = "kubernetes.deployments.list"
	PermKubernetesDeploymentsScale = "kubernetes.deployments.scale"

	// Kubernetes permissions — metrics (Phase 2)
	PermKubernetesMetricsView = "kubernetes.metrics.view"

	// Kubernetes permissions — nodes (Phase 2)
	PermKubernetesNodesView   = "kubernetes.nodes.view"
	PermKubernetesNodesCordon = "kubernetes.nodes.cordon"
	PermKubernetesNodesDrain  = "kubernetes.nodes.drain"

	// Kubernetes permissions — quotas (Phase 2)
	PermKubernetesQuotaView = "kubernetes.quota.view"

	// Kubernetes permissions — namespaces
	PermKubernetesNamespacesList = "kubernetes.namespaces.list"

	// Kubernetes permissions — cronjobs
	PermKubernetesCronJobsList = "kubernetes.cronjobs.list"

	// Admin permissions
	PermAdminUsersManage = "admin.users.manage"
	PermAdminRBACManage  = "admin.rbac.manage"
	PermAdminAuditView   = "admin.audit.view"

	// ArgoCD permissions
	PermArgoCDAppsList     = "argocd.apps.list"     // viewer+
	PermArgoCDAppsView     = "argocd.apps.view"     // viewer+
	PermArgoCDAppsDiff     = "argocd.apps.diff"     // operator+
	PermArgoCDAppsSync     = "argocd.apps.sync"     // admin only
	PermArgoCDAppsRollback = "argocd.apps.rollback" // admin only
	PermArgoCDFreezeManage = "argocd.freeze.manage" // admin only

	// Helm permissions (Phase 4)
	PermHelmReleaseslist     = "helm.releases.list"
	PermHelmReleasesRollback = "helm.releases.rollback"

	// Phase 4 special roles — use entity.RoleSuperAdmin and entity.RoleOnCall
)

// defaultRoles returns the built-in role definitions.
func defaultRoles() []entity.Role {
	now := time.Now()
	viewerPerms := []string{
		PermKubernetesPodsList,
		PermKubernetesPodsGet,
		PermKubernetesPodsLogs,
		PermKubernetesPodsEvents,
		PermKubernetesMetricsView,
		PermKubernetesNodesView,
		PermKubernetesQuotaView,
		PermKubernetesNamespacesList,
		PermKubernetesDeploymentsList,
		PermKubernetesCronJobsList,
		PermArgoCDAppsList,
		PermArgoCDAppsView,
	}
	operatorPerms := append(viewerPerms,
		PermKubernetesPodsRestart,
		PermKubernetesDeploymentsScale,
		PermArgoCDAppsDiff,
		PermHelmReleaseslist,
	)
	adminPerms := append(operatorPerms,
		PermKubernetesPodsDelete,
		PermKubernetesNodesCordon,
		PermKubernetesNodesDrain,
		PermAdminUsersManage,
		PermAdminRBACManage,
		PermAdminAuditView,
		PermArgoCDAppsSync,
		PermArgoCDAppsRollback,
		PermArgoCDFreezeManage,
		PermHelmReleasesRollback,
	)
	onCallPerms := append(operatorPerms,
		PermArgoCDAppsRollback,
		PermKubernetesNodesDrain,
	)

	return []entity.Role{
		{
			Name:        entity.RoleViewer,
			DisplayName: "Viewer",
			Description: "Read-only access to Kubernetes resources",
			Rules:       flatPermsToRules(viewerPerms, "allow"),
			IsBuiltin:   true,
			Permissions: viewerPerms,
			CreatedAt:   now,
		},
		{
			Name:        entity.RoleOperator,
			DisplayName: "Operator",
			Description: "Can perform operational actions like restart and scale",
			Rules:       flatPermsToRules(operatorPerms, "allow"),
			IsBuiltin:   true,
			Permissions: operatorPerms,
			CreatedAt:   now,
		},
		{
			Name:        entity.RoleAdmin,
			DisplayName: "Admin",
			Description: "Full access to all features",
			Rules:       flatPermsToRules(adminPerms, "allow"),
			IsBuiltin:   true,
			Permissions: adminPerms,
			CreatedAt:   now,
		},
		{
			Name:        entity.RoleSuperAdmin,
			DisplayName: "Super Admin",
			Description: "Super admin — always allowed (config-driven)",
			Rules:       []entity.PolicyRule{{Modules: []string{"*"}, Resources: []string{"*"}, Actions: []string{"*"}, Clusters: []string{"*"}, Namespaces: []string{"*"}, Effect: "allow"}},
			IsBuiltin:   true,
			Permissions: adminPerms,
			CreatedAt:   now,
		},
		{
			Name:        entity.RoleOnCall,
			DisplayName: "On-Call",
			Description: "Emergency access — includes rollback and drain; auto-expires",
			Rules:       flatPermsToRules(onCallPerms, "allow"),
			IsBuiltin:   true,
			Permissions: onCallPerms,
			CreatedAt:   now,
		},
	}
}

// flatPermsToRules converts legacy flat-perm strings to PolicyRule list.
// Each flat perm "module.resource.action" becomes one rule.
func flatPermsToRules(perms []string, effect string) []entity.PolicyRule {
	rules := make([]entity.PolicyRule, 0, len(perms))
	for _, p := range perms {
		parts := splitPerm(p)
		if len(parts) != 3 {
			continue
		}
		rules = append(rules, entity.PolicyRule{
			Modules:    []string{parts[0]},
			Resources:  []string{parts[1]},
			Actions:    []string{parts[2]},
			Clusters:   []string{"*"},
			Namespaces: []string{"*"},
			Effect:     effect,
		})
	}
	return rules
}

func splitPerm(perm string) []string {
	parts := make([]string, 0, 3)
	start := 0
	for i, c := range perm {
		if c == '.' {
			parts = append(parts, perm[start:i])
			start = i + 1
		}
	}
	parts = append(parts, perm[start:])
	return parts
}
