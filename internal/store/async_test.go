package store

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeRequestStore embeds the Store interface (nil) and implements only the one
// method the async request logger calls.
type fakeRequestStore struct {
	Store
	mu       sync.Mutex
	calls    int
	failNext int
	batches  [][]RequestLog
}

func (f *fakeRequestStore) LogRequestBatch(_ context.Context, logs []RequestLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.failNext > 0 {
		f.failNext--
		return errors.New("write failed")
	}
	f.batches = append(f.batches, append([]RequestLog(nil), logs...))
	return nil
}

func (f *fakeRequestStore) delivered() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, b := range f.batches {
		total += len(b)
	}
	return total
}

func (f *fakeRequestStore) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestAsyncRequestLoggerDrainsOnClose(t *testing.T) {
	fake := &fakeRequestStore{}
	// Long flush interval so the only flush is the Close drain (deterministic).
	l := NewAsyncRequestLogger(fake, discardLogger(), NewLoggerConfig(100, time.Hour))
	for i := 0; i < 5; i++ {
		if !l.Log(RequestLog{RequestID: "r", Model: "m"}) {
			t.Fatalf("Log(%d) returned false unexpectedly", i)
		}
	}
	if err := l.Close(context.Background()); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if got := fake.delivered(); got != 5 {
		t.Fatalf("delivered = %d, want 5", got)
	}
}

func TestAsyncRequestLoggerRetriesOnceThenSucceeds(t *testing.T) {
	fake := &fakeRequestStore{failNext: 1}
	l := NewAsyncRequestLogger(fake, discardLogger(), NewLoggerConfig(100, time.Hour))
	l.Log(RequestLog{RequestID: "r"})
	if err := l.Close(context.Background()); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if got := fake.callCount(); got != 2 {
		t.Fatalf("LogRequestBatch calls = %d, want 2 (initial + one retry)", got)
	}
	if fake.delivered() != 1 {
		t.Fatal("entry should have been delivered on the retry")
	}
}

func TestAsyncRequestLoggerDropsAfterRetryFailure(t *testing.T) {
	fake := &fakeRequestStore{failNext: 2} // initial + retry both fail
	l := NewAsyncRequestLogger(fake, discardLogger(), NewLoggerConfig(100, time.Hour))
	l.Log(RequestLog{RequestID: "r"})
	if err := l.Close(context.Background()); err != nil {
		t.Fatalf("Close must not hang or error when a batch is dropped: %v", err)
	}
	if got := fake.callCount(); got != 2 {
		t.Fatalf("calls = %d, want 2 (initial + retry, then drop)", got)
	}
	if fake.delivered() != 0 {
		t.Fatal("entry should have been dropped after the retry also failed")
	}
}

func TestAsyncRequestLoggerDropsWhenBufferFull(t *testing.T) {
	// A store whose writes block until released keeps the run loop busy so the
	// buffer fills and Log reports the drop.
	release := make(chan struct{})
	blocking := &blockingRequestStore{release: release}
	l := NewAsyncRequestLogger(blocking, discardLogger(), NewLoggerConfig(1, time.Hour))

	dropped := false
	for i := 0; i < 500; i++ {
		if !l.Log(RequestLog{RequestID: "r"}) {
			dropped = true
			break
		}
	}
	close(release)
	_ = l.Close(context.Background())
	if !dropped {
		t.Fatal("expected at least one Log to be dropped while the buffer was full")
	}
}

type blockingRequestStore struct {
	Store
	release chan struct{}
	once    sync.Once
}

func (b *blockingRequestStore) LogRequestBatch(_ context.Context, _ []RequestLog) error {
	b.once.Do(func() { <-b.release })
	return nil
}
