package cluster

import "sync"

// UserContext tracks per-user cluster selection.
type UserContext struct {
	mu       sync.RWMutex
	selected map[int64]string // telegramUserID -> clusterName
	manager  Manager
}

// NewUserContext creates a new user context tracker.
func NewUserContext(mgr Manager) *UserContext {
	return &UserContext{
		selected: make(map[int64]string),
		manager:  mgr,
	}
}

// GetCluster returns the user's currently selected cluster name.
// Falls back to the default cluster if the user hasn't selected one.
func (uc *UserContext) GetCluster(userID int64) string {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	if name, ok := uc.selected[userID]; ok {
		return name
	}

	info, err := uc.manager.GetDefault()
	if err != nil {
		return ""
	}
	return info.Name
}

// SetCluster sets the user's active cluster.
func (uc *UserContext) SetCluster(userID int64, clusterName string) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.selected[userID] = clusterName
}
