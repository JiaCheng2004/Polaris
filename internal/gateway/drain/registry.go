// Package drain tracks live WebSocket streams so a graceful shutdown can send
// close frames and wait for them to finish, and exposes a draining flag the
// readiness probe consults so load balancers stop sending new traffic first.
package drain

import (
	"context"
	"sync"
	"sync/atomic"
)

// Registry holds drain callbacks for live long-lived streams (realtime audio,
// voice streaming, interpreting) and a process-wide draining flag.
type Registry struct {
	mu       sync.Mutex
	next     uint64
	conns    map[uint64]func(context.Context)
	draining atomic.Bool
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry {
	return &Registry{conns: make(map[uint64]func(context.Context))}
}

// Register adds a drain callback (which must send a close frame and break the
// read loop) and returns a deregister func to call when the stream ends. If the
// registry is already draining, the callback is invoked immediately (the stream
// opened during shutdown) and a no-op deregister is returned.
func (r *Registry) Register(closer func(context.Context)) func() {
	if closer == nil {
		return func() {}
	}
	r.mu.Lock()
	if r.draining.Load() {
		r.mu.Unlock()
		go closer(context.Background())
		return func() {}
	}
	id := r.next
	r.next++
	r.conns[id] = closer
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		delete(r.conns, id)
		r.mu.Unlock()
	}
}

// SetDraining flips the readiness flag so /ready reports not-ready without yet
// closing any streams (lets load balancers stop new traffic first).
func (r *Registry) SetDraining() { r.draining.Store(true) }

// Draining reports whether shutdown has begun.
func (r *Registry) Draining() bool { return r.draining.Load() }

// Count returns the number of live registered streams.
func (r *Registry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.conns)
}

// Drain marks the registry draining and invokes every drain callback, waiting up
// to ctx for them to finish. Safe to call once.
func (r *Registry) Drain(ctx context.Context) {
	r.draining.Store(true)
	r.mu.Lock()
	closers := make([]func(context.Context), 0, len(r.conns))
	for _, c := range r.conns {
		closers = append(closers, c)
	}
	r.conns = make(map[uint64]func(context.Context))
	r.mu.Unlock()

	if len(closers) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, c := range closers {
		wg.Add(1)
		go func(closer func(context.Context)) {
			defer wg.Done()
			closer(ctx)
		}(c)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
