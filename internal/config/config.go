// Package config holds all configuration types for the application.
package config

import "github.com/d9042n/telekube/pkg/logger"

// Settings is the root configuration struct.
type Settings struct {
	Telegram       TelegramConfig       `mapstructure:"telegram" validate:"required"`
	Clusters       []ClusterConfig      `mapstructure:"clusters"`
	ArgoCD         ArgoCDConfig         `mapstructure:"argocd"`
	Storage        StorageConfig        `mapstructure:"storage"`
	Modules        ModulesConfig        `mapstructure:"modules"`
	Server         ServerConfig         `mapstructure:"server"`
	Log            logger.Config        `mapstructure:"log"`
	RBAC           RBACConfig           `mapstructure:"rbac"`
	LeaderElection LeaderElectionConfig `mapstructure:"leader_election"`
	Approval       ApprovalConfig       `mapstructure:"approval"`
	Watcher        WatcherConfig        `mapstructure:"watcher"`
}

// TelegramConfig holds Telegram bot settings.
type TelegramConfig struct {
	Token        string  `mapstructure:"token" validate:"required"`
	AdminIDs     []int64 `mapstructure:"admin_ids" validate:"required,min=1"`
	WebhookURL   string  `mapstructure:"webhook_url"`
	AllowedChats []int64 `mapstructure:"allowed_chats"`
	RateLimit    int     `mapstructure:"rate_limit" validate:"min=1"`
}

// ClusterConfig holds Kubernetes cluster connection settings.
type ClusterConfig struct {
	Name        string `mapstructure:"name" validate:"required"`
	DisplayName string `mapstructure:"display_name"`
	Kubeconfig  string `mapstructure:"kubeconfig"`
	Context     string `mapstructure:"context"`
	InCluster   bool   `mapstructure:"in_cluster"`
	Default     bool   `mapstructure:"default"`
}

// StorageConfig holds storage backend settings.
type StorageConfig struct {
	Backend  string         `mapstructure:"backend" validate:"oneof=sqlite postgres"`
	SQLite   SQLiteConfig   `mapstructure:"sqlite"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
}

// SQLiteConfig holds SQLite connection settings.
type SQLiteConfig struct {
	Path string `mapstructure:"path"`
}

// PostgresConfig holds PostgreSQL connection settings.
type PostgresConfig struct {
	DSN             string `mapstructure:"dsn"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime_min"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Addr      string `mapstructure:"addr"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	DB        int    `mapstructure:"db"`
	TLSEnable bool   `mapstructure:"tls_enable"`
	PoolSize  int    `mapstructure:"pool_size"`
	OpTimeout int    `mapstructure:"op_timeout_ms"`
}

// ModulesConfig holds module toggle settings.
type ModulesConfig struct {
	Kubernetes ModuleToggle   `mapstructure:"kubernetes"`
	ArgoCD     ModuleToggle   `mapstructure:"argocd"`
	Watcher    ModuleToggle   `mapstructure:"watcher"`
	Approval   ModuleToggle   `mapstructure:"approval"`
	Briefing   BriefingToggle `mapstructure:"briefing"`
	AlertMgr   ModuleToggle   `mapstructure:"alertmanager"`
	Helm       ModuleToggle   `mapstructure:"helm"`
	Incident   ModuleToggle   `mapstructure:"incident"`
	Notify     ModuleToggle   `mapstructure:"notify"`
}

// ArgoCDConfig holds settings for one or more ArgoCD instances.
type ArgoCDConfig struct {
	Instances []ArgoCDInstanceConfig `mapstructure:"instances"`
	Insecure  bool                   `mapstructure:"insecure"`
	Timeout   string                 `mapstructure:"timeout"`
}

// ArgoCDInstanceConfig is a single ArgoCD server configuration.
type ArgoCDInstanceConfig struct {
	Name     string         `mapstructure:"name" validate:"required"`
	URL      string         `mapstructure:"url" validate:"required"`
	Auth     ArgoCDAuthConfig `mapstructure:"auth"`
	Clusters []string       `mapstructure:"clusters"` // Associated K8s cluster names
}

// ArgoCDAuthConfig holds auth settings for a single ArgoCD instance.
type ArgoCDAuthConfig struct {
	Type         string `mapstructure:"type"`          // "token" or "oauth"
	Token        string `mapstructure:"token"`         // For type=token
	TokenURL     string `mapstructure:"token_url"`     // For type=oauth
	ClientID     string `mapstructure:"client_id"`     // For type=oauth
	ClientSecret string `mapstructure:"client_secret"` // For type=oauth
}

// ModuleToggle represents a module that can be enabled/disabled.
type ModuleToggle struct {
	Enabled bool `mapstructure:"enabled"`
}

// BriefingToggle extends ModuleToggle with schedule configuration.
type BriefingToggle struct {
	Enabled  bool   `mapstructure:"enabled"`
	Schedule string `mapstructure:"schedule"`  // Cron expression e.g. "0 8 * * *"
	Timezone string `mapstructure:"timezone"`  // e.g. "Asia/Ho_Chi_Minh"
}

// ServerConfig holds HTTP server settings for health/metrics.
type ServerConfig struct {
	Port           int `mapstructure:"port"`
	ReadTimeoutMS  int `mapstructure:"read_timeout_ms"`
	WriteTimeoutMS int `mapstructure:"write_timeout_ms"`
	IdleTimeoutMS  int `mapstructure:"idle_timeout_ms"`
}

// RBACConfig holds RBAC settings.
type RBACConfig struct {
	DefaultRole string `mapstructure:"default_role"`
}

// LeaderElectionConfig holds leader election settings.
type LeaderElectionConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Namespace string `mapstructure:"namespace"` // K8s namespace for lease
}

// ApprovalConfig holds approval workflow settings.
type ApprovalConfig struct {
	Enabled       bool               `mapstructure:"enabled"`
	DefaultExpiry string             `mapstructure:"default_expiry"` // e.g. "30m"
	Rules         []ApprovalRuleConfig `mapstructure:"rules"`
}

// ApprovalRuleConfig defines a single approval rule.
type ApprovalRuleConfig struct {
	Action            string   `mapstructure:"action"`             // e.g. "argocd.apps.sync"
	Clusters          []string `mapstructure:"clusters"`           // e.g. ["prod-1"] or ["*"]
	RequiredApprovals int      `mapstructure:"required_approvals"` // e.g. 1
	ApproverRoles     []string `mapstructure:"approver_roles"`     // e.g. ["admin", "super-admin"]
}

// WatcherConfig holds watcher-specific settings.
type WatcherConfig struct {
	CustomRules []CustomAlertRuleConfig `mapstructure:"custom_rules"`
}

// CustomAlertRuleConfig defines a single custom alert rule.
type CustomAlertRuleConfig struct {
	Name        string                      `mapstructure:"name"`
	Description string                      `mapstructure:"description"`
	Severity    string                      `mapstructure:"severity"` // "info", "warning", "critical"
	Condition   CustomAlertConditionConfig  `mapstructure:"condition"`
	Scope       CustomAlertScopeConfig      `mapstructure:"scope"`
	Notify      CustomAlertNotifyConfig     `mapstructure:"notify"`
}

// CustomAlertConditionConfig specifies the condition to evaluate.
type CustomAlertConditionConfig struct {
	Type      string `mapstructure:"type"`      // "pod_restart_count", "pod_pending_duration", etc.
	Threshold int    `mapstructure:"threshold"` // numeric threshold
	Window    string `mapstructure:"window"`    // time window e.g. "5m"
	Resource  string `mapstructure:"resource"`  // for quota checks: "memory", "cpu"
}

// CustomAlertScopeConfig limits rule evaluation.
type CustomAlertScopeConfig struct {
	Clusters   []string `mapstructure:"clusters"`
	Namespaces []string `mapstructure:"namespaces"`
}

// CustomAlertNotifyConfig specifies alert delivery.
type CustomAlertNotifyConfig struct {
	Chats   []int64  `mapstructure:"chats"`
	Mention []string `mapstructure:"mention"`
}
