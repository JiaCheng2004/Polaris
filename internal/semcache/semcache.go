// Package semcache is Polaris's embedding semantic cache. It complements the
// exact (SHA-256) response cache with an L2 vector layer: request text is
// embedded and matched against recent entries by cosine similarity, so
// paraphrases hit the cache. Entries are namespaced by model, settings, project,
// and an epoch (so cross-project/model/settings lookups can never collide and a
// config change invalidates the set). The in-process index is bounded (LRU) and
// mutex-guarded — fixing the read-modify-write race of the old token-similarity
// index. When the embedder is unhealthy the cache degrades to exact-only.
package semcache

import (
	"context"
	"fmt"
	"time"
)

// Embedder turns text into vectors. Fingerprint identifies the embedding
// configuration so a change bumps the cache epoch.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Fingerprint() string
}

// ResponseStore persists cached response bodies (the gateway cache backend).
type ResponseStore interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

// MetricsSink receives semantic-cache metric updates (the gateway recorder
// satisfies it; semcache stays below the gateway layer).
type MetricsSink interface {
	IncCacheEvent(status, model string)
	ObserveSemanticSimilarity(score float64)
	AddCacheSavings(model string, usd float64)
	SetSemcacheDegraded(degraded bool)
}

type noopMetrics struct{}

func (noopMetrics) IncCacheEvent(string, string)      {}
func (noopMetrics) ObserveSemanticSimilarity(float64) {}
func (noopMetrics) AddCacheSavings(string, float64)   {}
func (noopMetrics) SetSemcacheDegraded(bool)          {}

// Verdict is the outcome of a semantic lookup.
type Verdict string

const (
	VerdictMiss        Verdict = "miss"
	VerdictHitSemantic Verdict = "hit-semantic"
	VerdictBypass      Verdict = "bypass"
	VerdictDegraded    Verdict = "degraded"
)

// Namespace isolates cache entries. Two requests share entries only if every
// field matches — enforcing cross-project, cross-model, cross-settings, and
// cross-epoch isolation.
type Namespace struct {
	ModelID      string
	SettingsHash string
	ProjectID    string
	Epoch        uint64
}

func (n Namespace) key() string {
	return fmt.Sprintf("%d\x00%s\x00%s\x00%s", n.Epoch, n.ProjectID, n.ModelID, n.SettingsHash)
}
