package semcache

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	degradeThreshold = 5 // consecutive embed failures → degrade to exact-only
	degradeCooldown  = 30 * time.Second
)

// Config holds semantic-cache tunables sourced from response_cache config.
type Config struct {
	Threshold       float64
	TTL             time.Duration
	MaxPerNamespace int
}

// Cache is the process-lifetime semantic cache. The in-process per-namespace
// index is authoritative for semantic matches; response bodies persist to the
// backend (shared across replicas for exact reuse).
type Cache struct {
	embedder Embedder
	backend  ResponseStore
	metrics  MetricsSink
	cfg      atomic.Pointer[Config]
	now      func() time.Time

	mu         sync.Mutex
	namespaces map[string]*nsIndex

	failMu        sync.Mutex
	failCount     int
	degradedUntil time.Time
}

// New builds a Cache. A nil embedder disables semantic lookups (Enabled=false).
func New(embedder Embedder, backend ResponseStore, metrics MetricsSink, cfg Config) *Cache {
	if metrics == nil {
		metrics = noopMetrics{}
	}
	c := &Cache{embedder: embedder, backend: backend, metrics: metrics, now: time.Now, namespaces: map[string]*nsIndex{}}
	c.cfg.Store(&cfg)
	return c
}

func (c *Cache) Reconfigure(cfg Config) { c.cfg.Store(&cfg) }

func (c *Cache) config() Config {
	if p := c.cfg.Load(); p != nil {
		return *p
	}
	return Config{}
}

// Enabled reports whether semantic lookups can run (embedder + backend wired).
func (c *Cache) Enabled() bool { return c != nil && c.embedder != nil && c.backend != nil }

// Fingerprint identifies the embedding configuration (folds into the epoch).
func (c *Cache) Fingerprint() string {
	if c == nil || c.embedder == nil {
		return ""
	}
	return c.embedder.Fingerprint()
}

func (c *Cache) index(nsKey string, create bool) *nsIndex {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.namespaces[nsKey]
	if idx == nil && create {
		idx = newNSIndex(c.config().MaxPerNamespace)
		c.namespaces[nsKey] = idx
	}
	return idx
}

// Lookup embeds the query, scans the namespace index, and returns the cached
// response body on a semantic hit. It never crosses namespaces (isolation).
func (c *Cache) Lookup(ctx context.Context, ns Namespace, queryText string) ([]byte, Verdict, error) {
	if !c.Enabled() || queryText == "" {
		return nil, VerdictBypass, nil
	}
	if c.degraded() {
		return nil, VerdictDegraded, nil
	}
	vecs, err := c.embedder.Embed(ctx, []string{queryText})
	if err != nil || len(vecs) == 0 {
		c.recordEmbedFailure()
		return nil, VerdictDegraded, err
	}
	c.recordEmbedSuccess()

	idx := c.index(ns.key(), false)
	if idx == nil {
		c.metrics.IncCacheEvent("miss", ns.ModelID)
		return nil, VerdictMiss, nil
	}
	storeKey, score := idx.best(normalize(vecs[0]), c.config().Threshold, c.now())
	if storeKey == "" {
		c.metrics.IncCacheEvent("miss", ns.ModelID)
		return nil, VerdictMiss, nil
	}
	raw, ok, err := c.backend.Get(ctx, storeKey)
	if err != nil || !ok {
		c.metrics.IncCacheEvent("miss", ns.ModelID)
		return nil, VerdictMiss, err
	}
	body, decErr := base64.StdEncoding.DecodeString(raw)
	if decErr != nil {
		c.metrics.IncCacheEvent("miss", ns.ModelID)
		return nil, VerdictMiss, nil
	}
	c.metrics.ObserveSemanticSimilarity(score)
	c.metrics.IncCacheEvent("hit-semantic", ns.ModelID)
	return body, VerdictHitSemantic, nil
}

// Store embeds the query, records the vector→storeKey mapping in the namespace
// index, and persists the response body under storeKey.
func (c *Cache) Store(ctx context.Context, ns Namespace, queryText, storeKey string, body []byte) error {
	if !c.Enabled() || queryText == "" || storeKey == "" || len(body) == 0 || c.degraded() {
		return nil
	}
	vecs, err := c.embedder.Embed(ctx, []string{queryText})
	if err != nil || len(vecs) == 0 {
		c.recordEmbedFailure()
		return err
	}
	c.recordEmbedSuccess()

	cfg := c.config()
	var expires time.Time
	if cfg.TTL > 0 {
		expires = c.now().Add(cfg.TTL)
	}
	if err := c.backend.Set(ctx, storeKey, base64.StdEncoding.EncodeToString(body), cfg.TTL); err != nil {
		return err
	}
	c.index(ns.key(), true).add(normalize(vecs[0]), storeKey, expires)
	return nil
}

// Purge drops in-process namespace indexes matching the optional project and
// model filters (empty = all). Returns the number of namespaces removed.
func (c *Cache) Purge(project, model string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	removed := 0
	for k := range c.namespaces {
		if matchNSKey(k, project, model) {
			delete(c.namespaces, k)
			removed++
		}
	}
	return removed
}

// matchNSKey matches a namespace key (epoch\x00project\x00model\x00settings)
// against optional project/model filters.
func matchNSKey(k, project, model string) bool {
	parts := strings.Split(k, "\x00")
	if len(parts) < 4 {
		return project == "" && model == ""
	}
	if project != "" && parts[1] != project {
		return false
	}
	if model != "" && parts[2] != model {
		return false
	}
	return true
}

func (c *Cache) degraded() bool {
	c.failMu.Lock()
	defer c.failMu.Unlock()
	if c.degradedUntil.IsZero() {
		return false
	}
	if c.now().Before(c.degradedUntil) {
		return true
	}
	c.degradedUntil = time.Time{}
	c.failCount = 0
	return false
}

func (c *Cache) recordEmbedFailure() {
	c.failMu.Lock()
	defer c.failMu.Unlock()
	c.failCount++
	if c.failCount >= degradeThreshold {
		c.degradedUntil = c.now().Add(degradeCooldown)
		c.metrics.SetSemcacheDegraded(true)
	}
}

func (c *Cache) recordEmbedSuccess() {
	c.failMu.Lock()
	defer c.failMu.Unlock()
	c.failCount = 0
	if !c.degradedUntil.IsZero() {
		c.degradedUntil = time.Time{}
		c.metrics.SetSemcacheDegraded(false)
	}
}
