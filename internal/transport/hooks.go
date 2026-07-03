package transport

import "time"

// AttemptInfo describes one HTTP attempt for observers (the reliability manager
// and tracing). It is emitted once per attempt, including retries.
type AttemptInfo struct {
	Provider string
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
