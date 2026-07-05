package reliability

import (
	"math"
	"sync"
	"time"
)

const (
	// retryBudgetHalfLife decays the success/retry counters so the budget
	// reflects recent traffic.
	retryBudgetHalfLife = 10 * time.Second
	// retryBudgetMinTokens lets a low-traffic or cold gateway still perform a
	// reasonable number of failovers before the ratio constraint binds. Only a
	// sustained retry storm (many failovers, few successes) exhausts the budget.
	retryBudgetMinTokens = 100.0
)

// retryBudget bounds failover/retry attempts to a fraction of recent successes
// (default 20%) plus a floor, preventing retry storms from amplifying load
// during an outage. Safe for concurrent use.
type retryBudget struct {
	mu        sync.Mutex
	successes float64
	retries   float64
	ratio     float64
	last      time.Time
}

func newRetryBudget(ratio float64) *retryBudget {
	if ratio <= 0 {
		ratio = 0.2
	}
	return &retryBudget{ratio: ratio}
}

func (b *retryBudget) decayLocked(now time.Time) {
	if b.last.IsZero() {
		b.last = now
		return
	}
	elapsed := now.Sub(b.last)
	if elapsed <= 0 {
		return
	}
	decay := math.Exp2(-float64(elapsed) / float64(retryBudgetHalfLife))
	b.successes *= decay
	b.retries *= decay
	b.last = now
}

// recordSuccess replenishes the budget.
func (b *retryBudget) recordSuccess(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.decayLocked(now)
	b.successes++
}

// allowRetry reports whether one more retry/failover attempt is within budget,
// consuming a token if so.
func (b *retryBudget) allowRetry(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.decayLocked(now)
	if b.retries+1 > b.ratio*b.successes+retryBudgetMinTokens {
		return false
	}
	b.retries++
	return true
}

func (b *retryBudget) reconfigure(ratio float64) {
	if ratio <= 0 {
		ratio = 0.2
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ratio = ratio
}
