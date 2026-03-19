package kubernetes

import (
	"context"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// namespaceCache caches namespace lists per cluster.
type namespaceCache struct {
	mu    sync.RWMutex
	cache map[string]nsCacheEntry
	ttl   time.Duration
}

type nsCacheEntry struct {
	namespaces []string
	fetchedAt  time.Time
}

func newNamespaceCache() *namespaceCache {
	return &namespaceCache{
		cache: make(map[string]nsCacheEntry),
		ttl:   60 * time.Second,
	}
}

func (nc *namespaceCache) get(clusterName string) ([]string, bool) {
	nc.mu.RLock()
	defer nc.mu.RUnlock()

	entry, ok := nc.cache[clusterName]
	if !ok || time.Since(entry.fetchedAt) > nc.ttl {
		return nil, false
	}
	return entry.namespaces, true
}

func (nc *namespaceCache) set(clusterName string, namespaces []string) {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	nc.cache[clusterName] = nsCacheEntry{
		namespaces: namespaces,
		fetchedAt:  time.Now(),
	}
}

// getNamespaces returns cached or fresh namespace list for a cluster.
func (m *Module) getNamespaces(ctx context.Context, clusterName string) ([]string, error) {
	// Check cache
	if ns, ok := m.nsCache.get(clusterName); ok {
		return ns, nil
	}

	// Fetch from cluster
	clientSet, err := m.cluster.ClientSet(clusterName)
	if err != nil {
		return nil, err
	}

	nsList, err := clientSet.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var names []string
	for _, ns := range nsList.Items {
		names = append(names, ns.Name)
	}

	m.nsCache.set(clusterName, names)
	return names, nil
}
