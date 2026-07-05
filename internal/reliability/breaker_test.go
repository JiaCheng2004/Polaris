package reliability

import (
	"testing"
	"time"
)

func TestBreakerOpensAfterThreshold(t *testing.T) {
	b := newBreaker(BreakerConfig{ErrorRate: 0.5, MinSamples: 20, OpenFor: 30 * time.Second, HalfOpenProbes: 3})
	now := time.Unix(0, 0)

	// Below min samples: never opens even at 100% error.
	for i := 0; i < 19; i++ {
		transitioned, _, _ := b.record(false, 1.0, i+1, now)
		if transitioned {
			t.Fatalf("opened at %d samples, want ≥20", i+1)
		}
	}
	if b.currentState() != Closed {
		t.Fatalf("state = %v, want closed", b.currentState())
	}
	// At min samples with error rate ≥ threshold: opens.
	transitioned, from, to := b.record(false, 0.6, 20, now)
	if !transitioned || from != Closed || to != Open {
		t.Fatalf("transition = %v %v->%v, want closed->open", transitioned, from, to)
	}
}

func TestBreakerDoesNotOpenBelowErrorRate(t *testing.T) {
	b := newBreaker(BreakerConfig{})
	now := time.Unix(0, 0)
	transitioned, _, _ := b.record(true, 0.3, 100, now)
	if transitioned || b.currentState() != Closed {
		t.Fatalf("opened at 30%% error rate, want closed")
	}
}

func TestBreakerHalfOpenRecovery(t *testing.T) {
	b := newBreaker(BreakerConfig{ErrorRate: 0.5, MinSamples: 5, OpenFor: 10 * time.Second, HalfOpenProbes: 2})
	now := time.Unix(0, 0)
	b.record(false, 1.0, 5, now) // open

	// While open, allow rejects until OpenFor elapses.
	if allowed, _, _, _ := b.allow(now.Add(5 * time.Second)); allowed {
		t.Fatal("allowed while open before OpenFor")
	}
	// After OpenFor: transitions to half-open and admits a probe.
	allowed, transitioned, from, to := b.allow(now.Add(11 * time.Second))
	if !allowed || !transitioned || from != Open || to != HalfOpen {
		t.Fatalf("half-open transition = %v %v %v->%v", allowed, transitioned, from, to)
	}
	// Second probe admitted (HalfOpenProbes=2), third rejected.
	if allowed, _, _, _ := b.allow(now.Add(11 * time.Second)); !allowed {
		t.Fatal("second probe rejected")
	}
	if allowed, _, _, _ := b.allow(now.Add(11 * time.Second)); allowed {
		t.Fatal("third probe admitted beyond HalfOpenProbes")
	}
	// Two successful probes close the breaker.
	b.record(true, 0, 5, now.Add(12*time.Second))
	if b.currentState() != HalfOpen {
		t.Fatalf("closed after 1 probe success, want still half-open")
	}
	transitioned, _, to = b.record(true, 0, 5, now.Add(12*time.Second))
	if !transitioned || to != Closed {
		t.Fatalf("did not close after 2 probe successes: %v %v", transitioned, to)
	}
}

func TestBreakerHalfOpenReopensOnFailure(t *testing.T) {
	b := newBreaker(BreakerConfig{ErrorRate: 0.5, MinSamples: 5, OpenFor: 10 * time.Second, HalfOpenProbes: 3})
	now := time.Unix(0, 0)
	b.record(false, 1.0, 5, now)       // open
	b.allow(now.Add(11 * time.Second)) // -> half-open probe
	transitioned, from, to := b.record(false, 1.0, 5, now.Add(12*time.Second))
	if !transitioned || from != HalfOpen || to != Open {
		t.Fatalf("half-open failure did not reopen: %v %v->%v", transitioned, from, to)
	}
}

func TestBreakerStateString(t *testing.T) {
	for state, want := range map[BreakerState]string{Closed: "closed", Open: "open", HalfOpen: "half_open"} {
		if got := state.String(); got != want {
			t.Fatalf("%d.String() = %q, want %q", state, got, want)
		}
	}
}
