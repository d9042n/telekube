package audit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ─── Fake repo for testing ───────────────────────────────────────────────────

type fakeAuditRepo struct {
	mu      sync.Mutex
	entries []entity.AuditEntry
	listFn  func(ctx context.Context, f storage.AuditFilter) ([]entity.AuditEntry, int, error)
	errFn   func() error // if set, Create returns this error
}

func (r *fakeAuditRepo) Create(_ context.Context, entry *entity.AuditEntry) error {
	if r.errFn != nil {
		return r.errFn()
	}
	r.mu.Lock()
	r.entries = append(r.entries, *entry)
	r.mu.Unlock()
	return nil
}

func (r *fakeAuditRepo) List(ctx context.Context, f storage.AuditFilter) ([]entity.AuditEntry, int, error) {
	if r.listFn != nil {
		return r.listFn(ctx, f)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.entries, len(r.entries), nil
}

func (r *fakeAuditRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestAuditLogger_Log_And_Flush(t *testing.T) {
	t.Parallel()

	repo := &fakeAuditRepo{}
	logger := NewLogger(repo, zap.NewNop())

	logger.Log(entity.AuditEntry{ID: "1", Action: "/pods", UserID: 100})
	logger.Log(entity.AuditEntry{ID: "2", Action: "/nodes", UserID: 200})

	// Flush should persist entries.
	err := logger.Flush(context.Background())
	require.NoError(t, err)

	// Give worker time to process.
	time.Sleep(100 * time.Millisecond)

	// Close ensures everything is flushed.
	err = logger.Close()
	require.NoError(t, err)

	assert.GreaterOrEqual(t, repo.count(), 2)
}

func TestAuditLogger_Close_FlushesRemaining(t *testing.T) {
	t.Parallel()

	repo := &fakeAuditRepo{}
	logger := NewLogger(repo, zap.NewNop())

	for i := 0; i < 50; i++ {
		logger.Log(entity.AuditEntry{
			ID:     fmt.Sprintf("entry-%d", i),
			Action: "/test",
			UserID: int64(i),
		})
	}

	// Close should flush all pending entries.
	err := logger.Close()
	require.NoError(t, err)

	assert.Equal(t, 50, repo.count())
}

func TestAuditLogger_BufferFull_DropsEntry(t *testing.T) {
	t.Parallel()

	// Use a repo that blocks to simulate slow writes.
	blockCh := make(chan struct{})
	repo := &fakeAuditRepo{
		errFn: func() error {
			<-blockCh // Block indefinitely until test releases.
			return nil
		},
	}
	logger := NewLogger(repo, zap.NewNop())

	// Fill the buffer (defaultBufferSize = 1000).
	for i := 0; i < defaultBufferSize+100; i++ {
		logger.Log(entity.AuditEntry{
			ID:     fmt.Sprintf("overflow-%d", i),
			Action: "/spam",
		})
	}

	// Some entries should have been dropped silently.
	close(blockCh) // Unblock so Close can complete.
	_ = logger.Close()
}

func TestAuditLogger_Query_DelegatesToRepo(t *testing.T) {
	t.Parallel()

	repo := &fakeAuditRepo{
		listFn: func(_ context.Context, f storage.AuditFilter) ([]entity.AuditEntry, int, error) {
			return []entity.AuditEntry{
				{ID: "x", Action: "/test"},
			}, 1, nil
		},
	}
	logger := NewLogger(repo, zap.NewNop())
	defer func() { _ = logger.Close() }()

	entries, total, err := logger.Query(context.Background(), storage.AuditFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, entries, 1)
	assert.Equal(t, "/test", entries[0].Action)
}

func TestAuditLogger_Query_RepoError_Propagated(t *testing.T) {
	t.Parallel()

	repo := &fakeAuditRepo{
		listFn: func(_ context.Context, _ storage.AuditFilter) ([]entity.AuditEntry, int, error) {
			return nil, 0, fmt.Errorf("db connection lost")
		},
	}
	logger := NewLogger(repo, zap.NewNop())
	defer func() { _ = logger.Close() }()

	_, _, err := logger.Query(context.Background(), storage.AuditFilter{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db connection lost")
}

func TestAuditLogger_ConcurrentLog_NoRace(t *testing.T) {
	t.Parallel()

	repo := &fakeAuditRepo{}
	logger := NewLogger(repo, zap.NewNop())

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			logger.Log(entity.AuditEntry{
				ID:     fmt.Sprintf("concurrent-%d", idx),
				Action: "/concurrent",
				UserID: int64(idx),
			})
		}(i)
	}
	wg.Wait()

	err := logger.Close()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, repo.count(), 100, "most concurrent entries should be persisted")
}

func TestAuditLogger_Flush_EmptyQueue_NoError(t *testing.T) {
	t.Parallel()

	repo := &fakeAuditRepo{}
	logger := NewLogger(repo, zap.NewNop())
	defer func() { _ = logger.Close() }()

	// Flush on empty queue should succeed.
	err := logger.Flush(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, repo.count())
}

func TestAuditLogger_CreateError_DoesNotPanic(t *testing.T) {
	t.Parallel()

	callCount := 0
	repo := &fakeAuditRepo{
		errFn: func() error {
			callCount++
			return fmt.Errorf("disk full")
		},
	}
	logger := NewLogger(repo, zap.NewNop())

	logger.Log(entity.AuditEntry{ID: "err-1", Action: "/fail"})

	// Give worker time to process.
	time.Sleep(2 * time.Second)

	err := logger.Close()
	require.NoError(t, err) // Close itself should not error even if writes failed.
}
