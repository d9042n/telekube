package watcher

import (
	"context"
	"fmt"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// fakeNotifier captures sent alerts for testing.
type fakeNotifier struct {
	sendFunc func(chatID int64, text string, markup interface{}) error
}

func (f *fakeNotifier) SendAlert(chatID int64, text string, markup *telebot.ReplyMarkup) error {
	if f.sendFunc != nil {
		return f.sendFunc(chatID, text, markup)
	}
	return nil
}

// nopAuditLogger is a no-op audit logger.
type nopAuditLogger struct{}

func (n *nopAuditLogger) Log(entry entity.AuditEntry)                      {}
func (n *nopAuditLogger) Query(_ context.Context, _ storage.AuditFilter) ([]entity.AuditEntry, int, error) {
	return nil, 0, nil
}
func (n *nopAuditLogger) Flush(_ context.Context) error { return nil }
func (n *nopAuditLogger) Close() error                  { return nil }

// noopLogger returns a no-op zap.Logger for test silence.
func noopLogger() *zap.Logger {
	return zap.NewNop()
}

// fakeClusterManager is a minimal Manager for node watcher tests.
type fakeClusterManager struct{}

func (f *fakeClusterManager) List() []entity.ClusterInfo                                     { return nil }
func (f *fakeClusterManager) Get(name string) (*entity.ClusterInfo, error)                   { return nil, fmt.Errorf("not found") }
func (f *fakeClusterManager) GetDefault() (*entity.ClusterInfo, error)                       { return nil, fmt.Errorf("no default") }
func (f *fakeClusterManager) ClientSet(clusterName string) (kubernetes.Interface, error)     { return nil, fmt.Errorf("no client") }
func (f *fakeClusterManager) MetricsClient(clusterName string) (metricsv.Interface, error)   { return nil, fmt.Errorf("no metrics") }
func (f *fakeClusterManager) DynamicClient(clusterName string) (dynamic.Interface, error)    { return nil, fmt.Errorf("no dynamic") }
func (f *fakeClusterManager) HealthCheck(_ context.Context) map[string]entity.HealthStatus   { return nil }
func (f *fakeClusterManager) RESTConfig(_ string) (*rest.Config, error)                       { return nil, fmt.Errorf("no rest config") }
func (f *fakeClusterManager) Close() error                                                    { return nil }
