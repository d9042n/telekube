// Package audit provides non-blocking audit logging.
package audit

import (
	"context"
	"sync"
	"time"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"go.uber.org/zap"
)

const (
	defaultBufferSize = 1000
	flushInterval     = time.Second
	flushBatchSize    = 100
)

// Logger provides non-blocking audit logging.
type Logger interface {
	Log(entry entity.AuditEntry)
	Query(ctx context.Context, filter storage.AuditFilter) ([]entity.AuditEntry, int, error)
	Flush(ctx context.Context) error
	Close() error
}

type auditLogger struct {
	entries chan entity.AuditEntry
	repo    storage.AuditRepository
	logger  *zap.Logger
	wg      sync.WaitGroup
	done    chan struct{}
}

// NewLogger creates a new buffered audit logger.
func NewLogger(repo storage.AuditRepository, logger *zap.Logger) Logger {
	l := &auditLogger{
		entries: make(chan entity.AuditEntry, defaultBufferSize),
		repo:    repo,
		logger:  logger,
		done:    make(chan struct{}),
	}

	l.wg.Add(1)
	go l.worker()

	return l
}

// Log sends an audit entry to the background worker (non-blocking).
func (l *auditLogger) Log(entry entity.AuditEntry) {
	select {
	case l.entries <- entry:
	default:
		l.logger.Warn("audit log buffer full, dropping entry",
			zap.String("action", entry.Action),
			zap.Int64("user_id", entry.UserID),
		)
	}
}

// Query retrieves audit entries with filters.
func (l *auditLogger) Query(ctx context.Context, filter storage.AuditFilter) ([]entity.AuditEntry, int, error) {
	return l.repo.List(ctx, filter)
}

// Flush drains all pending entries to storage.
func (l *auditLogger) Flush(ctx context.Context) error {
	for {
		select {
		case entry := <-l.entries:
			if err := l.repo.Create(ctx, &entry); err != nil {
				l.logger.Error("failed to flush audit entry",
					zap.String("action", entry.Action),
					zap.Error(err),
				)
			}
		default:
			return nil
		}
	}
}

// Close stops the background worker and flushes remaining entries.
func (l *auditLogger) Close() error {
	close(l.done)
	l.wg.Wait()
	return l.Flush(context.Background())
}

func (l *auditLogger) worker() {
	defer l.wg.Done()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]entity.AuditEntry, 0, flushBatchSize)

	for {
		select {
		case entry := <-l.entries:
			batch = append(batch, entry)
			if len(batch) >= flushBatchSize {
				l.writeBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				l.writeBatch(batch)
				batch = batch[:0]
			}
		case <-l.done:
			// Drain remaining entries
			for {
				select {
				case entry := <-l.entries:
					batch = append(batch, entry)
				default:
					if len(batch) > 0 {
						l.writeBatch(batch)
					}
					return
				}
			}
		}
	}
}

func (l *auditLogger) writeBatch(batch []entity.AuditEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := range batch {
		if err := l.repo.Create(ctx, &batch[i]); err != nil {
			l.logger.Error("failed to write audit entry",
				zap.String("id", batch[i].ID),
				zap.String("action", batch[i].Action),
				zap.Error(err),
			)
		}
	}
}
