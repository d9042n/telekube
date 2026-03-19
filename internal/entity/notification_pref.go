package entity

// NotificationPreference stores per-user alert preferences.
type NotificationPreference struct {
	UserID          int64    `json:"user_id"`
	MinSeverity     string   `json:"min_severity"`     // "info", "warning", "critical"
	MutedAlerts     []string `json:"muted_alerts"`     // alert names to silence
	MutedClusters   []string `json:"muted_clusters"`   // clusters to silence
	QuietHoursStart *string  `json:"quiet_hours_start"` // "22:00"
	QuietHoursEnd   *string  `json:"quiet_hours_end"`   // "08:00"
	Timezone        string   `json:"timezone"`          // "Asia/Ho_Chi_Minh"
}

// AlertRule describes a user-defined watch/alert rule defined in config.
type AlertRule struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Condition   AlertCondition    `json:"condition"`
	Scope       AlertScope        `json:"scope"`
	Severity    string            `json:"severity"` // "info", "warning", "critical"
	Notify      AlertNotify       `json:"notify"`
}

// AlertCondition specifies the condition to evaluate.
type AlertCondition struct {
	Type      string `json:"type"`      // "pod_restart_count", "pod_pending_duration", etc.
	Threshold int    `json:"threshold"` // numeric threshold (restarts, percent)
	Window    string `json:"window"`    // time window e.g. "5m"
	Resource  string `json:"resource"`  // for quota checks: "memory", "cpu"
}

// AlertScope limits rule evaluation to specific clusters/namespaces.
type AlertScope struct {
	Clusters   []string `json:"clusters"`   // ["prod-1"] or ["*"]
	Namespaces []string `json:"namespaces"` // ["production"] or ["*"]
}

// AlertNotify specifies how to deliver the alert.
type AlertNotify struct {
	Chats   []int64  `json:"chats"`   // empty = all configured chats
	Mention []string `json:"mention"` // usernames to @mention
}
