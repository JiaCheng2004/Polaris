package semcache

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"testing"
	"time"
)

// bagEmbedder is a deterministic bag-of-words embedder for tests: identical text
// → identical vectors (cosine 1.0), shared tokens → partial similarity.
type bagEmbedder struct {
	dim  int
	err  error
	fail bool
}

func (e *bagEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if e.fail {
		return nil, errors.New("embedder down")
	}
	if e.err != nil {
		return nil, e.err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, e.dim)
		for _, tok := range strings.Fields(strings.ToLower(t)) {
			h := fnv.New32a()
			_, _ = h.Write([]byte(tok))
			v[h.Sum32()%uint32(e.dim)] += 1
		}
		out[i] = v
	}
	return out, nil
}

func (e *bagEmbedder) Fingerprint() string { return "bag" }

type mapBackend struct {
	mu sync.Mutex
	m  map[string]string
}

func newMapBackend() *mapBackend { return &mapBackend{m: map[string]string{}} }

func (b *mapBackend) Get(_ context.Context, key string) (string, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.m[key]
	return v, ok, nil
}

func (b *mapBackend) Set(_ context.Context, key, value string, _ time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.m[key] = value
	return nil
}

func newTestCache(t *testing.T, embedder Embedder, threshold float64) *Cache {
	t.Helper()
	return New(embedder, newMapBackend(), nil, Config{Threshold: threshold, MaxPerNamespace: 100})
}

func TestSemcacheHitAndMiss(t *testing.T) {
	c := newTestCache(t, &bagEmbedder{dim: 64}, 0.85)
	ns := Namespace{ModelID: "openai/gpt-4o", ProjectID: "p1", Epoch: 1}
	ctx := context.Background()

	if err := c.Store(ctx, ns, "what is the capital of france", "k1", []byte(`{"a":1}`)); err != nil {
		t.Fatalf("store: %v", err)
	}

	// Identical query → hit.
	body, verdict, _ := c.Lookup(ctx, ns, "what is the capital of france")
	if verdict != VerdictHitSemantic || string(body) != `{"a":1}` {
		t.Fatalf("identical lookup = %q, %s", body, verdict)
	}

	// Near paraphrase (shared tokens) → hit above 0.85.
	if _, v, _ := c.Lookup(ctx, ns, "what is the capital of france today"); v != VerdictHitSemantic {
		t.Fatalf("near-paraphrase verdict = %s (want hit)", v)
	}

	// Unrelated query → miss.
	if _, v, _ := c.Lookup(ctx, ns, "tell me a joke about cats"); v != VerdictMiss {
		t.Fatalf("unrelated verdict = %s (want miss)", v)
	}
}

func TestSemcacheIsolation(t *testing.T) {
	c := newTestCache(t, &bagEmbedder{dim: 64}, 0.85)
	ctx := context.Background()
	base := Namespace{ModelID: "openai/gpt-4o", SettingsHash: "s1", ProjectID: "p1", Epoch: 1}
	query := "shared question text here"
	if err := c.Store(ctx, base, query, "k1", []byte(`secret`)); err != nil {
		t.Fatalf("store: %v", err)
	}

	// Every dimension that differs MUST miss — security-grade isolation.
	for _, other := range []Namespace{
		{ModelID: "anthropic/claude", SettingsHash: "s1", ProjectID: "p1", Epoch: 1}, // model
		{ModelID: "openai/gpt-4o", SettingsHash: "s2", ProjectID: "p1", Epoch: 1},    // settings
		{ModelID: "openai/gpt-4o", SettingsHash: "s1", ProjectID: "p2", Epoch: 1},    // project
		{ModelID: "openai/gpt-4o", SettingsHash: "s1", ProjectID: "p1", Epoch: 2},    // epoch
	} {
		if body, v, _ := c.Lookup(ctx, other, query); v == VerdictHitSemantic {
			t.Fatalf("isolation breach: ns %+v hit with body %q", other, body)
		}
	}
	// Same namespace still hits.
	if _, v, _ := c.Lookup(ctx, base, query); v != VerdictHitSemantic {
		t.Fatal("same namespace should still hit")
	}
}

func TestSemcacheDegradation(t *testing.T) {
	embedder := &bagEmbedder{dim: 64}
	c := New(embedder, newMapBackend(), nil, Config{Threshold: 0.85, MaxPerNamespace: 100})
	ctx := context.Background()
	ns := Namespace{ModelID: "m", Epoch: 1}

	// Store a healthy entry.
	_ = c.Store(ctx, ns, "hello world", "k1", []byte(`x`))

	// Embedder fails repeatedly → degrade to exact-only.
	embedder.fail = true
	for i := 0; i < degradeThreshold; i++ {
		if _, v, _ := c.Lookup(ctx, ns, "hello world"); v != VerdictDegraded {
			t.Fatalf("attempt %d verdict = %s (want degraded)", i, v)
		}
	}
	// Now in the degraded window: even a would-be hit returns degraded without embedding.
	if _, v, _ := c.Lookup(ctx, ns, "hello world"); v != VerdictDegraded {
		t.Fatalf("post-threshold verdict = %s (want degraded)", v)
	}
}

func TestSemcacheLRUEviction(t *testing.T) {
	c := New(&bagEmbedder{dim: 64}, newMapBackend(), nil, Config{Threshold: 0.99, MaxPerNamespace: 3})
	ctx := context.Background()
	ns := Namespace{ModelID: "m", Epoch: 1}
	for i := 0; i < 10; i++ {
		_ = c.Store(ctx, ns, fmt.Sprintf("unique query number %d alpha", i), fmt.Sprintf("k%d", i), []byte("v"))
	}
	if got := c.index(ns.key(), false).size(); got != 3 {
		t.Fatalf("index size = %d, want 3 (LRU-capped)", got)
	}
}

func TestSemcachePurge(t *testing.T) {
	c := newTestCache(t, &bagEmbedder{dim: 64}, 0.85)
	ctx := context.Background()
	_ = c.Store(ctx, Namespace{ModelID: "openai/gpt-4o", ProjectID: "p1", Epoch: 1}, "q one", "k1", []byte("v"))
	_ = c.Store(ctx, Namespace{ModelID: "openai/gpt-4o", ProjectID: "p2", Epoch: 1}, "q two", "k2", []byte("v"))
	if n := c.Purge("p1", ""); n != 1 {
		t.Fatalf("purge p1 removed %d, want 1", n)
	}
	if _, v, _ := c.Lookup(ctx, Namespace{ModelID: "openai/gpt-4o", ProjectID: "p1", Epoch: 1}, "q one"); v != VerdictMiss {
		t.Fatal("purged namespace should miss")
	}
	if _, v, _ := c.Lookup(ctx, Namespace{ModelID: "openai/gpt-4o", ProjectID: "p2", Epoch: 1}, "q two"); v != VerdictHitSemantic {
		t.Fatal("unpurged namespace should still hit")
	}
	if n := c.Purge("", ""); n != 1 {
		t.Fatalf("purge-all removed %d, want 1 (p2 remaining)", n)
	}
}

func BenchmarkSemcacheLookup5K(b *testing.B) {
	c := New(&fixedEmbedder{dim: 1536}, newMapBackend(), nil, Config{Threshold: 0.99, MaxPerNamespace: 5000})
	ctx := context.Background()
	ns := Namespace{ModelID: "m", Epoch: 1}
	for i := 0; i < 5000; i++ {
		_ = c.Store(ctx, ns, fmt.Sprintf("q%d", i), fmt.Sprintf("k%d", i), []byte("v"))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = c.Lookup(ctx, ns, "qX")
	}
}

// fixedEmbedder returns a spread-out vector per text (for the 5K bench).
type fixedEmbedder struct{ dim int }

func (e *fixedEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, e.dim)
		h := fnv.New64a()
		_, _ = h.Write([]byte(t))
		seed := h.Sum64()
		for d := 0; d < e.dim; d++ {
			v[d] = float32((seed>>(d%64))&1) + float32(d%7)
		}
		out[i] = v
	}
	return out, nil
}

func (e *fixedEmbedder) Fingerprint() string { return "fixed" }
