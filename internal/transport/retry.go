package transport

import (
	"context"
	"errors"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"time"
)

const defaultMaxBackoff = 5 * time.Second

// RetryPolicy governs the transport's per-request retry loop. MaxAttempts is
// taken from each provider's configuration exactly as before (1 == no retry),
// so migrating a provider onto the transport never changes how many times it
// retries. The behavior improvements over the old per-provider loops are:
// full-jitter backoff, honoring Retry-After, and never returning a drained
// response when a backoff sleep is interrupted.
type RetryPolicy struct {
	MaxAttempts       int
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	RespectRetryAfter bool
}

func (p RetryPolicy) attempts() int {
	if p.MaxAttempts < 1 {
		return 1
	}
	return p.MaxAttempts
}

func (p RetryPolicy) initialDelay() time.Duration {
	if p.InitialDelay <= 0 {
		return 200 * time.Millisecond
	}
	return p.InitialDelay
}

func (p RetryPolicy) maxDelay() time.Duration {
	if p.MaxDelay <= 0 {
		return defaultMaxBackoff
	}
	return p.MaxDelay
}

// backoff returns a full-jitter exponential backoff delay for the given attempt
// (1-indexed): a random value in [0, min(maxDelay, initial<<(attempt-1))].
// Jitter decorrelates concurrent retriers so a recovering upstream is not hit
// by a synchronized thundering herd.
func (p RetryPolicy) backoff(attempt int) time.Duration {
	initial := p.initialDelay()
	limit := p.maxDelay()
	ceil := initial
	for i := 1; i < attempt; i++ {
		ceil *= 2
		if ceil >= limit {
			ceil = limit
			break
		}
	}
	if ceil <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(ceil) + 1))
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func retryableTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// backoffSleep is the sleep used between retry attempts; it is a package var so
// tests can deterministically simulate an interrupted backoff (the B2 fix).
var backoffSleep = sleepWithContext

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// parseRetryAfter parses an HTTP Retry-After header value, supporting both the
// delta-seconds and HTTP-date forms. It returns the delay and whether a value
// was present and valid.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	if t, err := http.ParseTime(value); err == nil {
		d := time.Until(t)
		if now := now; !now.IsZero() {
			d = t.Sub(now)
		}
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}
