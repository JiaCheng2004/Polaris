package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeAuditStore struct {
	Store
	mu       sync.Mutex
	calls    int
	failNext int
	notImpl  bool
	events   []AuditEvent
}

func (f *fakeAuditStore) LogAuditEventBatch(_ context.Context, events []AuditEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.notImpl {
		return ErrNotImplemented
	}
	if f.failNext > 0 {
		f.failNext--
		return errors.New("write failed")
	}
	f.events = append(f.events, events...)
	return nil
}

func (f *fakeAuditStore) delivered() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

func TestAsyncAuditLoggerDrainsOnClose(t *testing.T) {
	fake := &fakeAuditStore{}
	l := NewAsyncAuditLogger(fake, discardLogger(), NewLoggerConfig(100, time.Hour))
	for i := 0; i < 5; i++ {
		l.Log(AuditEvent{Kind: "k"})
	}
	if err := l.Close(context.Background()); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if got := fake.delivered(); got != 5 {
		t.Fatalf("delivered = %d, want 5", got)
	}
}

func TestAsyncAuditLoggerLogAfterCloseNoPanic(t *testing.T) {
	fake := &fakeAuditStore{}
	l := NewAsyncAuditLogger(fake, discardLogger(), NewLoggerConfig(100, time.Hour))
	if err := l.Close(context.Background()); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	// Previously Close did close(entries), so this send would panic. It must not.
	_ = l.Log(AuditEvent{Kind: "late"})
}

func TestAsyncAuditLoggerNotImplementedNotDropped(t *testing.T) {
	fake := &fakeAuditStore{notImpl: true}
	sink := &countingSink{}
	l := NewAsyncAuditLogger(fake, discardLogger(), NewLoggerConfig(100, time.Hour))
	l.SetMetrics(sink)
	l.Log(AuditEvent{Kind: "k"})
	if err := l.Close(context.Background()); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if got := sink.count("audit/write_failed"); got != 0 {
		t.Fatalf("ErrNotImplemented counted as a drop (%d); it is not a real drop", got)
	}
}

func TestAsyncAuditLoggerNilSafe(t *testing.T) {
	l := NewAsyncAuditLogger(nil, discardLogger(), NewLoggerConfig(100, time.Hour))
	if l != nil {
		t.Fatal("NewAsyncAuditLogger(nil store) should return nil")
	}
	if l.Log(AuditEvent{}) {
		t.Fatal("nil logger Log should return false")
	}
	if err := l.Close(context.Background()); err != nil {
		t.Fatalf("nil logger Close = %v", err)
	}
}
