package keyboard

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// CallbackStore maps short keys to full callback data strings.
// Telegram limits callback_data to 64 bytes, but pod/namespace/cluster
// combinations can easily exceed that. This store keeps a short hash
// as the callback_data and resolves it back to the full string.
type CallbackStore struct {
	mu   sync.RWMutex
	data map[string]string
}

// global is the package-level singleton shared across all modules.
var (
	globalOnce  sync.Once
	globalStore *CallbackStore
)

// GlobalStore returns the shared CallbackStore singleton.
func GlobalStore() *CallbackStore {
	globalOnce.Do(func() {
		globalStore = &CallbackStore{data: make(map[string]string)}
	})
	return globalStore
}

// NewCallbackStore creates a new callback data store.
func NewCallbackStore() *CallbackStore {
	return GlobalStore()
}

// Store saves a full data string and returns a short key (max 15 chars).
// Telegram limits callback_data to 64 bytes total, but telebot prepends
// \f<unique>| (up to ~27 bytes for longest unique IDs like "k8s_node_uncordon_confirm").
// So the data portion must stay under ~36 bytes to be safe.
func (s *CallbackStore) Store(fullData string) string {
	const maxDataBytes = 36
	if len(fullData) <= maxDataBytes {
		return fullData
	}

	hash := sha256.Sum256([]byte(fullData))
	key := "cb_" + hex.EncodeToString(hash[:6]) // cb_ + 12 hex = 15 chars
	s.mu.Lock()
	s.data[key] = fullData
	s.mu.Unlock()
	return key
}

// Resolve returns the full data string for a short key.
// If the key is not found (i.e. it was already the full data), returns the key itself.
func (s *CallbackStore) Resolve(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if full, ok := s.data[key]; ok {
		return full
	}
	return key // Not a short key — return as-is
}
