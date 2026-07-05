package transport

import (
	"sync/atomic"
	"time"
)

// AttemptInfo describes one HTTP attempt for observers (the reliability manager
// and tracing). It is emitted once per attempt, including retries.
type AttemptInfo struct {
	Provider string // display name, e.g. "OpenAI"
	Slug     string // stable low-cardinality key, e.g. "openai" (matches config/registry)
	Attempt  int
	Method   string
	Path     string
	Status   int
	Err      error
	Latency  time.Duration
}

// AttemptHook observes a single attempt. Hooks must not block; they feed
// health/latency accounting and span annotation.
type AttemptHook func(AttemptInfo)

// observers are process-lifetime AttemptHooks invoked for every attempt on every
// Client, in addition to that Client's own Options.Hooks. The reliability
// Manager registers here once at startup so it observes all provider traffic
// without threading a hook through every provider constructor. Copy-on-write via
// an atomic pointer keeps report() lock-free on the hot path.
var observers atomic.Pointer[[]AttemptHook]

// AddObserver registers a process-lifetime observer invoked for every attempt on
// every Client. Intended to be called during startup (before serving); safe to
// call concurrently.
func AddObserver(hook AttemptHook) {
	if hook == nil {
		return
	}
	for {
		old := observers.Load()
		var next []AttemptHook
		if old != nil {
			next = make([]AttemptHook, len(*old), len(*old)+1)
			copy(next, *old)
		}
		next = append(next, hook)
		if observers.CompareAndSwap(old, &next) {
			return
		}
	}
}

// ResetObservers removes all registered observers. Tests only.
func ResetObservers() { observers.Store(nil) }

func loadObservers() []AttemptHook {
	if p := observers.Load(); p != nil {
		return *p
	}
	return nil
}
