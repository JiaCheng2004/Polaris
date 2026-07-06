package reliability

import (
	"context"
	"errors"
	"time"
)

// ErrNoAttempts is returned by Hedge when given no attempt functions.
var ErrNoAttempts = errors.New("hedge: no attempts")

type hedgeOutcome[T any] struct {
	value T
	err   error
	index int
}

// Hedge races attempt functions with staggered starts to cut tail latency:
// attempts[0] starts immediately; each later attempt starts after delay if no
// attempt has won yet, or immediately when a running attempt fails. The first
// successful result wins and every other attempt's context is cancelled (so
// losers stop before their usage is recorded — the caller bills only the
// winner). Use ONLY for idempotent operations; never for job submits or other
// non-idempotent writes. If all attempts fail, the last error is returned.
func Hedge[T any](ctx context.Context, delay time.Duration, attempts []func(context.Context) (T, error)) (T, int, error) {
	var zero T
	if len(attempts) == 0 {
		return zero, -1, ErrNoAttempts
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan hedgeOutcome[T], len(attempts))
	started, pending := 0, 0
	var lastErr error

	start := func() {
		i := started
		started++
		pending++
		go func() {
			v, err := attempts[i](ctx)
			results <- hedgeOutcome[T]{value: v, err: err, index: i}
		}()
	}

	start()
	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			if started < len(attempts) {
				start()
				timer.Reset(delay)
			}
		case res := <-results:
			pending--
			if res.err == nil {
				cancel() // winner: stop the losers
				return res.value, res.index, nil
			}
			lastErr = res.err
			if started < len(attempts) {
				// A running attempt failed — start the next candidate now rather
				// than waiting for the hedge delay.
				start()
				timer.Reset(delay)
			} else if pending == 0 {
				return zero, -1, lastErr
			}
		case <-ctx.Done():
			return zero, -1, ctx.Err()
		}
	}
}
