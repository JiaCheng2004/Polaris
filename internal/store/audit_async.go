package store

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

type AsyncAuditLogger struct {
	store         Store
	logger        *slog.Logger
	metrics       atomic.Pointer[UsageDropSink]
	entries       chan AuditEvent
	flushInterval time.Duration
	batchSize     int
	stop          chan struct{}
	done          chan struct{}
	drainCtx      context.Context
	once          sync.Once
}

func NewAsyncAuditLogger(store Store, logger *slog.Logger, cfg LoggerConfig) *AsyncAuditLogger {
	if store == nil {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}

	bufferSize := cfg.BufferSize
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	flushInterval := cfg.FlushInterval
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}
	batchSize := bufferSize / 4
	if batchSize < 1 {
		batchSize = 1
	}
	if batchSize > 100 {
		batchSize = 100
	}

	l := &AsyncAuditLogger{
		store:         store,
		logger:        logger,
		entries:       make(chan AuditEvent, bufferSize),
		flushInterval: flushInterval,
		batchSize:     batchSize,
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	go l.run()
	return l
}

// SetMetrics attaches a sink for dropped-row accounting (atomic; wired once).
func (l *AsyncAuditLogger) SetMetrics(sink UsageDropSink) {
	if l == nil {
		return
	}
	l.metrics.Store(&sink)
}

func (l *AsyncAuditLogger) recordDropped(reason string, n int) {
	if l == nil {
		return
	}
	p := l.metrics.Load()
	if p == nil {
		return
	}
	sink := *p
	if sink == nil {
		return
	}
	for i := 0; i < n; i++ {
		sink.IncUsageDropped("audit", reason)
	}
}

func (l *AsyncAuditLogger) Log(entry AuditEvent) bool {
	if l == nil {
		return false
	}
	select {
	case l.entries <- entry:
		return true
	default:
		l.logger.Warn("audit event dropped because buffer is full", "kind", entry.Kind)
		l.recordDropped("buffer_full", 1)
		return false
	}
}

func (l *AsyncAuditLogger) Close(ctx context.Context) error {
	if l == nil {
		return nil
	}
	// A dedicated stop channel (not close(entries)) means a late Log() after Close
	// never panics on a closed channel.
	l.once.Do(func() {
		l.drainCtx = ctx
		close(l.stop)
	})
	select {
	case <-l.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *AsyncAuditLogger) run() {
	defer close(l.done)

	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	var batch []AuditEvent

	flush := func(ctx context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := l.flush(ctx, batch); err != nil {
			l.logger.Warn("dropping audit batch after retries", "error", err, "count", len(batch))
			l.recordDropped("write_failed", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry := <-l.entries:
			batch = append(batch, entry)
			if len(batch) >= l.batchSize {
				flush(context.Background())
			}
		case <-ticker.C:
			flush(context.Background())
		case <-l.stop:
			drainCtx := l.drainCtx
			if drainCtx == nil {
				drainCtx = context.Background()
			}
			for {
				select {
				case entry := <-l.entries:
					batch = append(batch, entry)
				default:
					flush(drainCtx)
					return
				}
			}
		}
	}
}

func (l *AsyncAuditLogger) flush(ctx context.Context, batch []AuditEvent) error {
	var err error
	for attempt := 1; attempt <= loggerMaxFlushAttempts; attempt++ {
		writeCtx, cancel := context.WithTimeout(ctx, loggerFlushTimeout)
		err = l.store.LogAuditEventBatch(writeCtx, batch)
		cancel()
		if err == nil || errors.Is(err, ErrNotImplemented) {
			// ErrNotImplemented: the store has no audit table — not a real drop.
			return nil
		}
		if attempt < loggerMaxFlushAttempts {
			l.logger.Warn("audit batch write failed, retrying", "attempt", attempt, "count", len(batch), "error", err)
			if !sleepCtx(ctx, loggerBackoff(attempt)) {
				break
			}
		}
	}
	return err
}
