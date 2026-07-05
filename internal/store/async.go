package store

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// UsageDropSink counts dropped usage/audit rows so that drops are observable
// (never silent). The gateway metrics.Recorder satisfies it; this keeps the
// store below the gateway layer.
type UsageDropSink interface {
	IncUsageDropped(kind, reason string)
}

const (
	loggerFlushTimeout     = 10 * time.Second
	loggerMaxFlushAttempts = 3
	loggerBackoffBase      = 50 * time.Millisecond
)

type AsyncRequestLogger struct {
	store         Store
	logger        *slog.Logger
	metrics       atomic.Pointer[UsageDropSink]
	entries       chan RequestLog
	flushInterval time.Duration
	batchSize     int
	stop          chan struct{}
	done          chan struct{}
	drainCtx      context.Context
	once          sync.Once
}

func NewAsyncRequestLogger(store Store, logger *slog.Logger, cfg LoggerConfig) *AsyncRequestLogger {
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
	batchSize := bufferSize
	if batchSize > 100 {
		batchSize = 100
	}

	l := &AsyncRequestLogger{
		store:         store,
		logger:        logger,
		entries:       make(chan RequestLog, bufferSize),
		flushInterval: flushInterval,
		batchSize:     batchSize,
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	go l.run()
	return l
}

// SetMetrics attaches a sink for dropped-row accounting. Safe to call after
// construction (atomic); wired once from main.go's shared recorder.
func (l *AsyncRequestLogger) SetMetrics(sink UsageDropSink) {
	l.metrics.Store(&sink)
}

func (l *AsyncRequestLogger) recordDropped(reason string, n int) {
	p := l.metrics.Load()
	if p == nil {
		return
	}
	sink := *p
	if sink == nil {
		return
	}
	for i := 0; i < n; i++ {
		sink.IncUsageDropped("request", reason)
	}
}

func (l *AsyncRequestLogger) Log(entry RequestLog) bool {
	select {
	case l.entries <- entry:
		return true
	default:
		l.logger.Warn("usage log dropped because buffer is full", "request_id", entry.RequestID, "model", entry.Model)
		l.recordDropped("buffer_full", 1)
		return false
	}
}

func (l *AsyncRequestLogger) Close(ctx context.Context) error {
	l.once.Do(func() {
		// drainCtx is read by run() only after it observes stop closed; the
		// channel close provides the happens-before, so no lock is needed.
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

func (l *AsyncRequestLogger) run() {
	defer close(l.done)

	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	var batch []RequestLog

	flush := func(ctx context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := l.flush(ctx, batch); err != nil {
			l.logger.Error("dropping usage log batch after retries", "error", err, "batch_size", len(batch))
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

// flush writes a batch with bounded retry-with-backoff. Each attempt is bounded
// by loggerFlushTimeout (and by ctx), so a hung store cannot stall shutdown.
func (l *AsyncRequestLogger) flush(ctx context.Context, batch []RequestLog) error {
	var err error
	for attempt := 1; attempt <= loggerMaxFlushAttempts; attempt++ {
		writeCtx, cancel := context.WithTimeout(ctx, loggerFlushTimeout)
		err = l.store.LogRequestBatch(writeCtx, batch)
		cancel()
		if err == nil {
			return nil
		}
		if attempt < loggerMaxFlushAttempts {
			l.logger.Warn("usage log batch write failed, retrying", "attempt", attempt, "batch_size", len(batch), "error", err)
			if !sleepCtx(ctx, loggerBackoff(attempt)) {
				break
			}
		}
	}
	return err
}

func loggerBackoff(attempt int) time.Duration {
	d := loggerBackoffBase << (attempt - 1)
	if d > 2*time.Second {
		d = 2 * time.Second
	}
	return d
}

// sleepCtx sleeps for d unless ctx is cancelled first; it returns false if ctx
// was cancelled (so callers stop retrying).
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
