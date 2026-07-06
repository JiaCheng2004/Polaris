package drain

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRegistryDrainInvokesClosers(t *testing.T) {
	r := NewRegistry()
	var closed atomic.Int32
	_ = r.Register(func(context.Context) { closed.Add(1) })
	if r.Count() != 1 {
		t.Fatalf("Count = %d, want 1", r.Count())
	}
	r.Drain(context.Background())
	if closed.Load() != 1 {
		t.Fatalf("closer invoked %d times, want 1", closed.Load())
	}
	if !r.Draining() {
		t.Fatal("registry not marked draining after Drain")
	}
}

func TestRegistryDeregister(t *testing.T) {
	r := NewRegistry()
	var closed atomic.Int32
	dereg := r.Register(func(context.Context) { closed.Add(1) })
	dereg()
	if r.Count() != 0 {
		t.Fatalf("Count after deregister = %d, want 0", r.Count())
	}
	r.Drain(context.Background())
	if closed.Load() != 0 {
		t.Fatal("deregistered closer was invoked")
	}
}

func TestRegistryRegisterDuringDraining(t *testing.T) {
	r := NewRegistry()
	r.SetDraining()
	done := make(chan struct{})
	_ = r.Register(func(context.Context) { close(done) })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("closer registered during draining was not invoked")
	}
}

func TestRegistrySetDraining(t *testing.T) {
	r := NewRegistry()
	if r.Draining() {
		t.Fatal("new registry should not be draining")
	}
	r.SetDraining()
	if !r.Draining() {
		t.Fatal("SetDraining did not flip the flag")
	}
}

func TestRegistryDrainBoundedByContext(t *testing.T) {
	r := NewRegistry()
	// A closer that blocks until its context is cancelled.
	_ = r.Register(func(ctx context.Context) { <-ctx.Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	r.Drain(ctx)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Drain took %v, should honor the 50ms ctx", elapsed)
	}
}

func TestRegistryNilCloser(t *testing.T) {
	r := NewRegistry()
	dereg := r.Register(nil)
	dereg() // must not panic
	if r.Count() != 0 {
		t.Fatalf("nil closer registered: Count = %d", r.Count())
	}
}
