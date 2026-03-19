// Package cluster manages connections to multiple Kubernetes clusters.
package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/pkg/kube"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Manager provides access to multiple Kubernetes clusters.
type Manager interface {
	List() []entity.ClusterInfo
	Get(name string) (*entity.ClusterInfo, error)
	GetDefault() (*entity.ClusterInfo, error)
	ClientSet(clusterName string) (kubernetes.Interface, error)
	MetricsClient(clusterName string) (metricsv.Interface, error)
	DynamicClient(clusterName string) (dynamic.Interface, error)
	HealthCheck(ctx context.Context) map[string]entity.HealthStatus
	Close() error
}

type clusterEntry struct {
	config  config.ClusterConfig
	info    entity.ClusterInfo
	clients *kube.Clients
}

type manager struct {
	clusters     map[string]*clusterEntry
	defaultName  string
	mu           sync.RWMutex
	logger       *zap.Logger
	done         chan struct{}
}

// NewManager creates a new cluster manager from config.
func NewManager(configs []config.ClusterConfig, logger *zap.Logger) Manager {
	m := &manager{
		clusters: make(map[string]*clusterEntry),
		logger:   logger,
		done:     make(chan struct{}),
	}

	for _, cfg := range configs {
		displayName := cfg.DisplayName
		if displayName == "" {
			displayName = cfg.Name
		}

		entry := &clusterEntry{
			config: cfg,
			info: entity.ClusterInfo{
				Name:        cfg.Name,
				DisplayName: displayName,
				InCluster:   cfg.InCluster,
				IsDefault:   cfg.Default,
				Status:      entity.HealthStatusUnknown,
			},
		}
		m.clusters[cfg.Name] = entry

		if cfg.Default {
			m.defaultName = cfg.Name
		}
	}

	// If no default set, use first cluster
	if m.defaultName == "" && len(configs) > 0 {
		m.defaultName = configs[0].Name
		if entry, ok := m.clusters[m.defaultName]; ok {
			entry.info.IsDefault = true
		}
	}

	// Start background health checker
	go m.healthChecker()

	return m
}

func (m *manager) List() []entity.ClusterInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]entity.ClusterInfo, 0, len(m.clusters))
	for _, entry := range m.clusters {
		infos = append(infos, entry.info)
	}
	return infos
}

func (m *manager) Get(name string) (*entity.ClusterInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.clusters[name]
	if !ok {
		return nil, fmt.Errorf("cluster %q not found", name)
	}
	info := entry.info
	return &info, nil
}

func (m *manager) GetDefault() (*entity.ClusterInfo, error) {
	if m.defaultName == "" {
		return nil, fmt.Errorf("no default cluster configured")
	}
	return m.Get(m.defaultName)
}

func (m *manager) ClientSet(clusterName string) (kubernetes.Interface, error) {
	clients, err := m.getOrCreateClients(clusterName)
	if err != nil {
		return nil, err
	}
	return clients.ClientSet, nil
}

func (m *manager) MetricsClient(clusterName string) (metricsv.Interface, error) {
	clients, err := m.getOrCreateClients(clusterName)
	if err != nil {
		return nil, err
	}
	return clients.MetricsClient, nil
}

func (m *manager) DynamicClient(clusterName string) (dynamic.Interface, error) {
	clients, err := m.getOrCreateClients(clusterName)
	if err != nil {
		return nil, err
	}
	return clients.DynamicClient, nil
}

// getOrCreateClients performs lazy initialization of K8s clients.
func (m *manager) getOrCreateClients(clusterName string) (*kube.Clients, error) {
	m.mu.RLock()
	entry, ok := m.clusters[clusterName]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("cluster %q not found", clusterName)
	}
	if entry.clients != nil {
		m.mu.RUnlock()
		return entry.clients, nil
	}
	m.mu.RUnlock()

	// Upgrade to write lock for initialization
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if entry.clients != nil {
		return entry.clients, nil
	}

	m.logger.Info("initializing K8s client",
		zap.String("cluster", clusterName),
		zap.Bool("in_cluster", entry.config.InCluster),
	)

	clients, err := kube.NewClients(kube.ClientConfig{
		Kubeconfig: entry.config.Kubeconfig,
		Context:    entry.config.Context,
		InCluster:  entry.config.InCluster,
	})
	if err != nil {
		entry.info.Status = entity.HealthStatusUnhealthy
		return nil, fmt.Errorf("creating K8s clients for cluster %q: %w", clusterName, err)
	}

	entry.clients = clients
	entry.info.Status = entity.HealthStatusHealthy
	return clients, nil
}

func (m *manager) HealthCheck(ctx context.Context) map[string]entity.HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make(map[string]entity.HealthStatus, len(m.clusters))
	for name, entry := range m.clusters {
		if entry.clients == nil {
			statuses[name] = entity.HealthStatusUnknown
			continue
		}

		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := entry.clients.ClientSet.Discovery().ServerVersion()
		cancel()

		if err != nil {
			statuses[name] = entity.HealthStatusUnhealthy
			entry.info.Status = entity.HealthStatusUnhealthy
		} else {
			statuses[name] = entity.HealthStatusHealthy
			entry.info.Status = entity.HealthStatusHealthy
		}

		_ = checkCtx // satisfy linter
	}
	return statuses
}

func (m *manager) Close() error {
	close(m.done)
	return nil
}

func (m *manager) healthChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			statuses := m.HealthCheck(ctx)
			cancel()

			for name, status := range statuses {
				if status == entity.HealthStatusUnhealthy {
					m.logger.Warn("cluster unhealthy",
						zap.String("cluster", name),
					)
				}
			}
		case <-m.done:
			return
		}
	}
}
