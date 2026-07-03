package transport

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestRetryPolicyDefaults(t *testing.T) {
	var zero RetryPolicy
	if zero.attempts() != 1 {
		t.Fatalf("attempts default = %d, want 1", zero.attempts())
	}
	if zero.initialDelay() != 200*time.Millisecond {
		t.Fatalf("initialDelay default = %v", zero.initialDelay())
	}
	if zero.maxDelay() != defaultMaxBackoff {
		t.Fatalf("maxDelay default = %v", zero.maxDelay())
	}
	set := RetryPolicy{MaxAttempts: 4, InitialDelay: time.Second, MaxDelay: 2 * time.Second}
	if set.attempts() != 4 || set.initialDelay() != time.Second || set.maxDelay() != 2*time.Second {
		t.Fatalf("explicit policy not honored: %+v", set)
	}
}

func TestRetryableStatus(t *testing.T) {
	for _, s := range []int{429, 500, 502, 503, 504} {
		if !retryableStatus(s) {
			t.Fatalf("retryableStatus(%d) = false", s)
		}
	}
	for _, s := range []int{200, 400, 401, 403, 404, 422} {
		if retryableStatus(s) {
			t.Fatalf("retryableStatus(%d) = true", s)
		}
	}
}

func TestRetryableTransportError(t *testing.T) {
	if retryableTransportError(nil) {
		t.Fatal("nil should not be retryable")
	}
	if !retryableTransportError(context.DeadlineExceeded) {
		t.Fatal("deadline exceeded should be retryable")
	}
	if !retryableTransportError(&net.OpError{Op: "dial", Err: errors.New("x")}) {
		t.Fatal("net.Error should be retryable")
	}
	if retryableTransportError(errors.New("plain")) {
		t.Fatal("plain error should not be retryable")
	}
}

func TestSleepWithContext(t *testing.T) {
	// Normal completion.
	if err := sleepWithContext(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("sleep err = %v", err)
	}
	// Zero delay, live context.
	if err := sleepWithContext(context.Background(), 0); err != nil {
		t.Fatalf("zero-delay sleep err = %v", err)
	}
	// Cancelled context, zero delay.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepWithContext(cancelled, 0); err == nil {
		t.Fatal("cancelled zero-delay sleep should error")
	}
	// Cancelled during a longer sleep.
	ctx, cancel2 := context.WithCancel(context.Background())
	go func() {
		time.Sleep(2 * time.Millisecond)
		cancel2()
	}()
	if err := sleepWithContext(ctx, time.Hour); err == nil {
		t.Fatal("interrupted sleep should error")
	}
}

func TestBackoffZeroCeiling(t *testing.T) {
	// A degenerate policy with zero initial delay still returns a valid (>=0) delay.
	p := RetryPolicy{InitialDelay: time.Nanosecond, MaxDelay: time.Nanosecond}
	if d := p.backoff(5); d < 0 {
		t.Fatalf("backoff = %v", d)
	}
}
