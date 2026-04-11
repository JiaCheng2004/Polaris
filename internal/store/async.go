package store

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type AsyncRequestLogger struct {
	store         Store
	logger        *slog.Logger
	entries       chan RequestLog
	flushInterval time.Duration
	batchSize     int
	stop          chan struct{}
	done          chan struct{}
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

func (l *AsyncRequestLogger) Log(entry RequestLog) bool {
	select {
	case l.entries <- entry:
		return true
	default:
		l.logger.Warn("usage log dropped because buffer is full", "request_id", entry.RequestID, "model", entry.Model)
		return false
	}
}

func (l *AsyncRequestLogger) Close(ctx context.Context) error {
	l.once.Do(func() {
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

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := l.flush(batch); err != nil {
			l.logger.Error("dropping usage log batch after retry failure", "error", err, "batch_size", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry := <-l.entries:
			batch = append(batch, entry)
			if len(batch) >= l.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-l.stop:
			for {
				select {
				case entry := <-l.entries:
					batch = append(batch, entry)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (l *AsyncRequestLogger) flush(batch []RequestLog) error {
	err := l.store.LogRequestBatch(context.Background(), batch)
	if err == nil {
		return nil
	}

	l.logger.Warn("usage log batch write failed, retrying once", "error", err, "batch_size", len(batch))
	if retryErr := l.store.LogRequestBatch(context.Background(), batch); retryErr != nil {
		return retryErr
	}
	return nil
}
