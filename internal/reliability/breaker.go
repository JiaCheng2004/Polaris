package reliability

import (
	"sync"
	"time"
)

// BreakerState is the circuit-breaker state. The zero value is Closed.
type BreakerState int

const (
	// Closed admits all traffic (healthy).
	Closed BreakerState = iota
	// Open rejects traffic until the open period elapses.
	Open
	// HalfOpen admits a limited number of probes to test recovery.
	HalfOpen
)

func (s BreakerState) String() string {
	switch s {
	case Open:
		return "open"
	case HalfOpen:
		return "half_open"
	default:
		return "closed"
	}
}

// BreakerConfig tunes a circuit breaker.
type BreakerConfig struct {
	ErrorRate      float64       // open when windowed error rate ≥ this
	MinSamples     int           // ...and at least this many samples in the window
	OpenFor        time.Duration // stay open this long before probing
	HalfOpenProbes int           // concurrent probes admitted while half-open
}

func (c BreakerConfig) withDefaults() BreakerConfig {
	if c.ErrorRate <= 0 {
		c.ErrorRate = 0.5
	}
	if c.MinSamples <= 0 {
		c.MinSamples = 20
	}
	if c.OpenFor <= 0 {
		c.OpenFor = 30 * time.Second
	}
	if c.HalfOpenProbes <= 0 {
		c.HalfOpenProbes = 3
	}
	return c
}

// breaker is one provider's circuit breaker. Safe for concurrent use.
type breaker struct {
	mu             sync.Mutex
	state          BreakerState
	openedAt       time.Time
	probesInflight int
	probeSuccesses int
	cfg            BreakerConfig
}

func newBreaker(cfg BreakerConfig) *breaker {
	return &breaker{cfg: cfg.withDefaults()}
}

// allow reports whether an attempt may proceed. When the open period has elapsed
// it transitions to half-open and admits a probe.
func (b *breaker) allow(now time.Time) (allowed bool, transitioned bool, from, to BreakerState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	from = b.state
	switch b.state {
	case Closed:
		allowed = true
	case Open:
		if now.Sub(b.openedAt) >= b.cfg.OpenFor {
			b.state = HalfOpen
			b.probesInflight = 1
			b.probeSuccesses = 0
			allowed = true
		}
	case HalfOpen:
		if b.probesInflight < b.cfg.HalfOpenProbes {
			b.probesInflight++
			allowed = true
		}
	}
	to = b.state
	return allowed, from != to, from, to
}

// record updates the breaker after an attempt completes, given the current
// windowed error rate and sample count.
func (b *breaker) record(success bool, errRate float64, samples int, now time.Time) (transitioned bool, from, to BreakerState) {
	b.mu.Lock()
	defer b.mu.Unlock()
	from = b.state
	switch b.state {
	case Closed:
		if samples >= b.cfg.MinSamples && errRate >= b.cfg.ErrorRate {
			b.state = Open
			b.openedAt = now
		}
	case HalfOpen:
		if success {
			b.probeSuccesses++
			if b.probeSuccesses >= b.cfg.HalfOpenProbes {
				b.state = Closed
				b.probesInflight = 0
				b.probeSuccesses = 0
			}
		} else {
			b.state = Open
			b.openedAt = now
			b.probesInflight = 0
			b.probeSuccesses = 0
		}
	case Open:
		// Transition out of Open happens in allow() once OpenFor elapses.
	}
	to = b.state
	return from != to, from, to
}

func (b *breaker) currentState() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *breaker) reconfigure(cfg BreakerConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cfg = cfg.withDefaults()
}
