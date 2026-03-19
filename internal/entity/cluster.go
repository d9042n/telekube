package entity

// ClusterInfo represents a registered Kubernetes cluster.
type ClusterInfo struct {
	Name        string       `json:"name"`
	DisplayName string       `json:"display_name"`
	InCluster   bool         `json:"in_cluster"`
	IsDefault   bool         `json:"is_default"`
	Status      HealthStatus `json:"status"`
}

// HealthStatus represents the health state of a component.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// StatusEmoji returns a display emoji for the health status.
func (h HealthStatus) Emoji() string {
	switch h {
	case HealthStatusHealthy:
		return "🟢"
	case HealthStatusUnhealthy:
		return "🔴"
	default:
		return "⚪"
	}
}
