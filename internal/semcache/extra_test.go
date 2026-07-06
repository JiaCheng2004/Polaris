package semcache

import (
	"context"
	"testing"
	"time"
)

func TestSemcacheReconfigureAndFingerprint(t *testing.T) {
	c := New(&bagEmbedder{dim: 8}, newMapBackend(), nil, Config{Threshold: 0.9})
	if c.Fingerprint() != "bag" {
		t.Fatalf("fingerprint = %q", c.Fingerprint())
	}
	c.Reconfigure(Config{Threshold: 0.5, MaxPerNamespace: 5})
	if c.config().Threshold != 0.5 {
		t.Fatalf("reconfigure threshold = %v", c.config().Threshold)
	}

	nilCache := New(nil, newMapBackend(), nil, Config{})
	if nilCache.Enabled() {
		t.Fatal("nil embedder should not be Enabled")
	}
	if nilCache.Fingerprint() != "" {
		t.Fatalf("nil-embedder fingerprint = %q", nilCache.Fingerprint())
	}
}

func TestSemcacheBypass(t *testing.T) {
	ctx := context.Background()
	c := New(&bagEmbedder{dim: 8}, newMapBackend(), nil, Config{Threshold: 0.9})
	if _, v, _ := c.Lookup(ctx, Namespace{ModelID: "m"}, ""); v != VerdictBypass {
		t.Fatalf("empty query verdict = %s", v)
	}
	// Disabled cache (no backend) bypasses and stores nothing.
	disabled := New(&bagEmbedder{dim: 8}, nil, nil, Config{})
	if _, v, _ := disabled.Lookup(ctx, Namespace{ModelID: "m"}, "hi"); v != VerdictBypass {
		t.Fatalf("disabled verdict = %s", v)
	}
	if err := disabled.Store(ctx, Namespace{ModelID: "m"}, "q", "k", []byte("v")); err != nil {
		t.Fatalf("disabled store err = %v", err)
	}
}

func TestSemcacheDegradedRecovery(t *testing.T) {
	ctx := context.Background()
	e := &bagEmbedder{dim: 8}
	c := New(e, newMapBackend(), nil, Config{Threshold: 0.9, MaxPerNamespace: 10})
	base := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return base }
	ns := Namespace{ModelID: "m", Epoch: 1}
	if err := c.Store(ctx, ns, "hello world", "k1", []byte("x")); err != nil {
		t.Fatalf("store: %v", err)
	}

	e.fail = true
	for i := 0; i < degradeThreshold; i++ {
		_, _, _ = c.Lookup(ctx, ns, "hello world")
	}
	if !c.degraded() {
		t.Fatal("expected degraded after repeated failures")
	}

	// Past the cooldown with a healthy embedder → recovers and hits.
	c.now = func() time.Time { return base.Add(degradeCooldown + time.Second) }
	e.fail = false
	if _, v, _ := c.Lookup(ctx, ns, "hello world"); v != VerdictHitSemantic {
		t.Fatalf("post-recovery verdict = %s (want hit)", v)
	}
}

func TestSemcacheTTLExpiry(t *testing.T) {
	ctx := context.Background()
	c := New(&bagEmbedder{dim: 8}, newMapBackend(), nil, Config{Threshold: 0.9, TTL: time.Minute, MaxPerNamespace: 10})
	base := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return base }
	ns := Namespace{ModelID: "m", Epoch: 1}
	_ = c.Store(ctx, ns, "hello world", "k1", []byte("x"))

	// Before expiry → hit.
	if _, v, _ := c.Lookup(ctx, ns, "hello world"); v != VerdictHitSemantic {
		t.Fatalf("pre-expiry verdict = %s", v)
	}
	// After TTL → the entry is skipped → miss.
	c.now = func() time.Time { return base.Add(2 * time.Minute) }
	if _, v, _ := c.Lookup(ctx, ns, "hello world"); v != VerdictMiss {
		t.Fatalf("post-expiry verdict = %s (want miss)", v)
	}
}

func TestSemcacheBackendMissAfterIndexHit(t *testing.T) {
	ctx := context.Background()
	backend := newMapBackend()
	c := New(&bagEmbedder{dim: 8}, backend, nil, Config{Threshold: 0.9, MaxPerNamespace: 10})
	ns := Namespace{ModelID: "m", Epoch: 1}
	_ = c.Store(ctx, ns, "hello world", "k1", []byte("x"))

	// Evict the body from the backend but leave the index entry: the vector
	// matches but the body is gone → miss (no stale serve).
	backend.mu.Lock()
	delete(backend.m, "k1")
	backend.mu.Unlock()
	if _, v, _ := c.Lookup(ctx, ns, "hello world"); v != VerdictMiss {
		t.Fatalf("backend-miss verdict = %s (want miss)", v)
	}
}

func TestSemcacheRefreshSameKey(t *testing.T) {
	ctx := context.Background()
	c := New(&bagEmbedder{dim: 8}, newMapBackend(), nil, Config{Threshold: 0.9, MaxPerNamespace: 10})
	ns := Namespace{ModelID: "m", Epoch: 1}
	_ = c.Store(ctx, ns, "hello world", "k1", []byte("x"))
	_ = c.Store(ctx, ns, "hello world again", "k1", []byte("y")) // same storeKey → refresh, not grow
	if got := c.index(ns.key(), false).size(); got != 1 {
		t.Fatalf("index size = %d, want 1 (refresh)", got)
	}
}

func TestSemcacheStoreEmbedFailure(t *testing.T) {
	c := New(&bagEmbedder{dim: 8, fail: true}, newMapBackend(), nil, Config{Threshold: 0.9})
	if err := c.Store(context.Background(), Namespace{ModelID: "m"}, "q", "k", []byte("v")); err == nil {
		t.Fatal("expected embed error to surface from Store")
	}
}

func TestNSIndexDimensionResetAndDefaults(t *testing.T) {
	idx := newNSIndex(0) // exercises the default-capacity branch
	if idx.capacity != 1000 {
		t.Fatalf("default capacity = %d", idx.capacity)
	}
	idx.add([]float32{1, 0, 0}, "k1", time.Time{})
	idx.add([]float32{1, 0}, "k2", time.Time{}) // different dim → index reset
	if idx.size() != 1 {
		t.Fatalf("after dimension reset size = %d, want 1", idx.size())
	}
	// Zero vector normalizes to itself; mismatched-length dot uses the shorter.
	if z := normalize([]float32{0, 0, 0}); z[0] != 0 {
		t.Fatalf("zero normalize = %v", z)
	}
	if got := dotF32([]float32{1, 1, 1, 1, 1}, []float32{1, 1}); got != 2 {
		t.Fatalf("mismatched dot = %v, want 2", got)
	}
}

func TestMatchNSKeyMalformed(t *testing.T) {
	if !matchNSKey("badkey", "", "") {
		t.Fatal("malformed key with no filter should match")
	}
	if matchNSKey("badkey", "p1", "") {
		t.Fatal("malformed key with a filter should not match")
	}
}

func TestNoopMetricsExercised(t *testing.T) {
	// New(nil metrics) uses noopMetrics; a full store+hit cycle must not panic.
	ctx := context.Background()
	c := New(&bagEmbedder{dim: 8}, newMapBackend(), nil, Config{Threshold: 0.9, MaxPerNamespace: 10})
	ns := Namespace{ModelID: "m", Epoch: 1}
	_ = c.Store(ctx, ns, "hello world", "k1", []byte("x"))
	if _, v, _ := c.Lookup(ctx, ns, "hello world"); v != VerdictHitSemantic {
		t.Fatalf("verdict = %s", v)
	}
}
