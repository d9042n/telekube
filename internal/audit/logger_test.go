package audit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockAuditRepo implements storage.AuditRepository for testing.
type mockAuditRepo struct {
	mu      sync.Mutex
	entries []entity.AuditEntry
}

func (m *mockAuditRepo) Create(_ context.Context, entry *entity.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, *entry)
	return nil
}

func (m *mockAuditRepo) List(_ context.Context, filter storage.AuditFilter) ([]entity.AuditEntry, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var filtered []entity.AuditEntry
	for _, e := range m.entries {
		if filter.UserID != nil && e.UserID != *filter.UserID {
			continue
		}
		if filter.Action != nil && e.Action != *filter.Action {
			continue
		}
		filtered = append(filtered, e)
	}

	total := len(filtered)
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	start := (page - 1) * pageSize
	if start >= total {
		return nil, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	return filtered[start:end], total, nil
}

func (m *mockAuditRepo) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

func TestLogger_Log_NonBlocking(t *testing.T) {
	t.Parallel()

	repo := &mockAuditRepo{}
	logger := NewLogger(repo, zap.NewNop())

	// Log should be non-blocking
	entry := entity.AuditEntry{
		ID:         "test-001",
		UserID:     100,
		Username:   "testuser",
		Action:     "pod.list",
		Status:     entity.AuditStatusSuccess,
		OccurredAt: time.Now().UTC(),
	}

	logger.Log(entry)

	// Give the background worker time to process
	time.Sleep(2 * time.Second)

	assert.GreaterOrEqual(t, repo.count(), 1)

	require.NoError(t, logger.Close())
}

func TestLogger_Log_MultipleEntries(t *testing.T) {
	t.Parallel()

	repo := &mockAuditRepo{}
	logger := NewLogger(repo, zap.NewNop())

	for i := 0; i < 10; i++ {
		logger.Log(entity.AuditEntry{
			ID:         "test-" + string(rune('A'+i)),
			UserID:     int64(100 + i),
			Action:     "pod.list",
			Status:     "success",
			OccurredAt: time.Now().UTC(),
		})
	}

	// Wait for flush
	time.Sleep(2 * time.Second)

	assert.Equal(t, 10, repo.count())

	require.NoError(t, logger.Close())
}

func TestLogger_Flush(t *testing.T) {
	t.Parallel()

	repo := &mockAuditRepo{}
	logger := NewLogger(repo, zap.NewNop())

	logger.Log(entity.AuditEntry{
		ID:         "flush-test",
		UserID:     100,
		Action:     "test.flush",
		Status:     "success",
		OccurredAt: time.Now().UTC(),
	})

	// Flush should drain
	err := logger.Flush(context.Background())
	require.NoError(t, err)

	require.NoError(t, logger.Close())
}

func TestLogger_Query(t *testing.T) {
	t.Parallel()

	repo := &mockAuditRepo{}
	repo.entries = []entity.AuditEntry{
		{ID: "1", UserID: 100, Action: "test", Status: "success"},
		{ID: "2", UserID: 200, Action: "test", Status: "success"},
	}

	logger := NewLogger(repo, zap.NewNop())
	defer func() { _ = logger.Close() }()

	entries, total, err := logger.Query(context.Background(), storage.AuditFilter{
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, entries, 2)
}

func TestLogger_Query_WithFilter(t *testing.T) {
	t.Parallel()

	repo := &mockAuditRepo{}
	repo.entries = []entity.AuditEntry{
		{ID: "1", UserID: 100, Action: "pod.list", Status: "success"},
		{ID: "2", UserID: 200, Action: "pod.restart", Status: "success"},
		{ID: "3", UserID: 100, Action: "pod.restart", Status: "denied"},
	}

	logger := NewLogger(repo, zap.NewNop())
	defer func() { _ = logger.Close() }()

	userID := int64(100)
	entries, total, err := logger.Query(context.Background(), storage.AuditFilter{
		UserID:   &userID,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, entries, 2)
}

func TestLogger_Close_DrainsRemaining(t *testing.T) {
	t.Parallel()

	repo := &mockAuditRepo{}
	logger := NewLogger(repo, zap.NewNop())

	// Log a few entries
	for i := 0; i < 5; i++ {
		logger.Log(entity.AuditEntry{
			ID:         "drain-" + string(rune('A'+i)),
			UserID:     int64(i),
			Action:     "drain.test",
			Status:     "success",
			OccurredAt: time.Now().UTC(),
		})
	}

	// Close should drain all
	require.NoError(t, logger.Close())

	assert.Equal(t, 5, repo.count())
}

func TestLogger_BufferFull_DropsEntries(t *testing.T) {
	t.Parallel()

	repo := &mockAuditRepo{}
	// Create logger with a very small buffer to test overflow
	l := &auditLogger{
		entries: make(chan entity.AuditEntry, 1), // tiny buffer
		repo:    repo,
		logger:  zap.NewNop(),
		done:    make(chan struct{}),
	}

	// Don't start the worker — entries won't be drained
	// Fill buffer
	l.Log(entity.AuditEntry{ID: "ok", Action: "test"})

	// This should be dropped (buffer full, no worker draining)
	l.Log(entity.AuditEntry{ID: "dropped", Action: "test"})

	// Only 1 should be in channel
	assert.Equal(t, 1, len(l.entries))
}
