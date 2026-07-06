package reliability

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestHedgeEmpty(t *testing.T) {
	_, _, err := Hedge[int](context.Background(), time.Millisecond, nil)
	if !errors.Is(err, ErrNoAttempts) {
		t.Fatalf("empty attempts err = %v, want ErrNoAttempts", err)
	}
}

func TestHedgeFastFirstNoExtra(t *testing.T) {
	var started atomic.Int32
	attempt := func(d time.Duration, v int) func(context.Context) (int, error) {
		return func(ctx context.Context) (int, error) {
			started.Add(1)
			select {
			case <-time.After(d):
				return v, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
	}
	// First is fast; hedge delay is long, so the second never starts.
	v, idx, err := Hedge(context.Background(), 200*time.Millisecond, []func(context.Context) (int, error){
		attempt(5*time.Millisecond, 1),
		attempt(5*time.Millisecond, 2),
	})
	if err != nil || v != 1 || idx != 0 {
		t.Fatalf("Hedge = %d,%d,%v, want 1,0,nil", v, idx, err)
	}
	time.Sleep(20 * time.Millisecond)
	if started.Load() != 1 {
		t.Fatalf("started %d attempts, want 1 (no hedge needed)", started.Load())
	}
}

func TestHedgeCutsTailLatency(t *testing.T) {
	slow := func(ctx context.Context) (string, error) {
		select {
		case <-time.After(500 * time.Millisecond):
			return "slow", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	fast := func(ctx context.Context) (string, error) {
		select {
		case <-time.After(10 * time.Millisecond):
			return "fast", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	start := time.Now()
	v, idx, err := Hedge(context.Background(), 20*time.Millisecond, []func(context.Context) (string, error){slow, fast})
	elapsed := time.Since(start)
	if err != nil || v != "fast" || idx != 1 {
		t.Fatalf("Hedge = %q,%d,%v, want fast,1,nil", v, idx, err)
	}
	if elapsed > 150*time.Millisecond {
		t.Fatalf("hedge took %v; should finish well before the 500ms slow attempt", elapsed)
	}
}

func TestHedgeFailoverBeforeDelay(t *testing.T) {
	failFast := func(context.Context) (int, error) { return 0, errors.New("boom") }
	succeed := func(context.Context) (int, error) { return 7, nil }
	start := time.Now()
	// Long hedge delay: a failing attempt should trigger the next immediately.
	v, idx, err := Hedge(context.Background(), time.Second, []func(context.Context) (int, error){failFast, succeed})
	if err != nil || v != 7 || idx != 1 {
		t.Fatalf("Hedge = %d,%d,%v, want 7,1,nil", v, idx, err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("hedge waited the full delay instead of retrying on failure")
	}
}

func TestHedgeAllFail(t *testing.T) {
	fail := func(context.Context) (int, error) { return 0, errors.New("nope") }
	_, _, err := Hedge(context.Background(), time.Millisecond, []func(context.Context) (int, error){fail, fail})
	if err == nil {
		t.Fatal("Hedge should return an error when all attempts fail")
	}
}

func TestHedgeWinnerCancelsLosers(t *testing.T) {
	var loserCancelled atomic.Bool
	winner := func(ctx context.Context) (int, error) {
		select {
		case <-time.After(30 * time.Millisecond):
			return 1, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	loser := func(ctx context.Context) (int, error) {
		<-ctx.Done() // blocks until cancelled by the winner
		loserCancelled.Store(true)
		return 0, ctx.Err()
	}
	_, idx, err := Hedge(context.Background(), 10*time.Millisecond, []func(context.Context) (int, error){winner, loser})
	if err != nil || idx != 0 {
		t.Fatalf("Hedge = %d,%v, want 0,nil", idx, err)
	}
	time.Sleep(20 * time.Millisecond)
	if !loserCancelled.Load() {
		t.Fatal("loser attempt was not cancelled after the winner returned")
	}
}
