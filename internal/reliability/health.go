package reliability

import (
	"sync"
	"time"
)

const (
	// healthWindowSeconds is the sliding window over which the error rate is
	// computed (a ring of 1-second buckets).
	healthWindowSeconds = 30
	// latencyAlpha is the EWMA smoothing factor for latency (α=0.2).
	latencyAlpha = 0.2
)

// errBucket counts attempts and provider-attributable failures in one 1s slot.
type errBucket struct {
	total int
	fail  int
}

// healthWindow tracks a provider's EWMA latency and a sliding error-rate window.
// It is safe for concurrent use.
type healthWindow struct {
	mu          sync.Mutex
	ewmaLatency float64 // milliseconds; 0 until the first sample
	buckets     [healthWindowSeconds]errBucket
	idx         int
	windowStart time.Time // start time of buckets[idx]
}

func newHealthWindow() *healthWindow { return &healthWindow{} }

// advanceLocked rolls the ring forward to now, clearing expired buckets. The
// caller must hold h.mu.
func (h *healthWindow) advanceLocked(now time.Time) {
	if h.windowStart.IsZero() {
		h.windowStart = now
		return
	}
	steps := int(now.Sub(h.windowStart) / time.Second)
	if steps <= 0 {
		return
	}
	if steps > len(h.buckets) {
		steps = len(h.buckets)
	}
	for i := 0; i < steps; i++ {
		h.idx = (h.idx + 1) % len(h.buckets)
		h.buckets[h.idx] = errBucket{}
	}
	h.windowStart = now
}

// record adds one outcome and returns the current window stats.
func (h *healthWindow) record(success bool, latency time.Duration, now time.Time) (samples int, errRate, ewmaMs float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.advanceLocked(now)
	b := &h.buckets[h.idx]
	b.total++
	if !success {
		b.fail++
	}
	lat := float64(latency.Milliseconds())
	if h.ewmaLatency == 0 {
		h.ewmaLatency = lat
	} else {
		h.ewmaLatency = latencyAlpha*lat + (1-latencyAlpha)*h.ewmaLatency
	}
	return h.statsLocked()
}

// stats returns the current window stats without recording anything.
func (h *healthWindow) stats(now time.Time) (samples int, errRate, ewmaMs float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.advanceLocked(now)
	return h.statsLocked()
}

func (h *healthWindow) statsLocked() (samples int, errRate, ewmaMs float64) {
	var fail int
	for _, b := range h.buckets {
		samples += b.total
		fail += b.fail
	}
	if samples > 0 {
		errRate = float64(fail) / float64(samples)
	}
	return samples, errRate, h.ewmaLatency
}

// score maps window stats onto a 0..1 health score (1 = perfectly healthy):
// success rate lightly penalized by latency. Used for the health-score gauge and
// (later) routing.
func healthScore(errRate, ewmaMs float64) float64 {
	success := 1 - errRate
	// Latency penalty saturates: ~0 below 500ms, growing toward 1 by ~10s.
	latPenalty := ewmaMs / (ewmaMs + 2000)
	score := success * (1 - 0.25*latPenalty)
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}
