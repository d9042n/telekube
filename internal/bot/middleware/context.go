package middleware

import (
	"gopkg.in/telebot.v3"
)

// Additional context key constants for middleware context injection.
const (
	clusterContextKey contextKey = "telekube_cluster"
)

// SetClusterContext injects the current cluster name into context.
func SetClusterContext(c telebot.Context, clusterName string) {
	c.Set(string(clusterContextKey), clusterName)
}

// GetClusterContext retrieves the current cluster from context.
func GetClusterContext(c telebot.Context) string {
	v := c.Get(string(clusterContextKey))
	if v == nil {
		return ""
	}
	name, ok := v.(string)
	if !ok {
		return ""
	}
	return name
}
