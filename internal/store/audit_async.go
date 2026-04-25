package store

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type AsyncAuditLogger struct {
	store         Store
	logger        *slog.Logger
	entries       chan AuditEvent
	flushInterval time.Duration
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

	l := &AsyncAuditLogger{
		store:         store,
		logger:        logger,
		entries:       make(chan AuditEvent, bufferSize),
		flushInterval: flushInterval,
	}
	go l.run()
	return l
}

func (l *AsyncAuditLogger) Log(entry AuditEvent) bool {
	if l == nil {
		return false
	}
	select {
	case l.entries <- entry:
		return true
	default:
		return false
	}
}

func (l *AsyncAuditLogger) Close(ctx context.Context) error {
	if l == nil {
		return nil
	}

	close(l.entries)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(l.flushInterval):
		return nil
	}
}

func (l *AsyncAuditLogger) run() {
	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	var batch []AuditEvent
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := l.store.LogAuditEventBatch(context.Background(), batch); err != nil && !errors.Is(err, ErrNotImplemented) {
			l.logger.Warn("audit batch flush failed", "error", err, "count", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry, ok := <-l.entries:
			if !ok {
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= cap(l.entries)/4 && cap(l.entries) > 0 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
