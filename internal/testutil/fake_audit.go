package testutil

import (
	"context"
	"sync"

	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
)

// FakeAuditLogger records audit entries in memory for assertion in tests.
// It is safe for concurrent use.
type FakeAuditLogger struct {
	mu      sync.Mutex
	entries []entity.AuditEntry
}

// NewFakeAuditLogger creates a no-op, in-memory audit logger.
func NewFakeAuditLogger() *FakeAuditLogger { return &FakeAuditLogger{} }

// Log records the entry in memory.
func (f *FakeAuditLogger) Log(entry entity.AuditEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entry)
}

// Query returns recorded entries (ignores filters for simplicity).
func (f *FakeAuditLogger) Query(_ context.Context, _ storage.AuditFilter) ([]entity.AuditEntry, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]entity.AuditEntry, len(f.entries))
	copy(out, f.entries)
	return out, len(out), nil
}

// Flush is a no-op in-memory implementation.
func (f *FakeAuditLogger) Flush(_ context.Context) error { return nil }

// Close is a no-op in-memory implementation.
func (f *FakeAuditLogger) Close() error { return nil }

// Entries returns a snapshot of all recorded entries.
func (f *FakeAuditLogger) Entries() []entity.AuditEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]entity.AuditEntry, len(f.entries))
	copy(out, f.entries)
	return out
}

// Clear resets the recorded entries (useful for test isolation).
func (f *FakeAuditLogger) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = nil
}

// Count returns the number of recorded entries.
func (f *FakeAuditLogger) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries)
}
