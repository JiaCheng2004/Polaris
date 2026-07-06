# Semantic Cache

Polaris's response cache has two layers:

- **L1 exact** — a SHA-256 hash of the request. Identical requests return the
  stored response. Always on when `response_cache.enabled: true`.
- **L2 semantic** — the request text is embedded and matched against recent
  entries by cosine similarity, so *paraphrases* hit the cache. Off by default.

Both are **disabled by default**. Enabling them never changes correctness — a
miss just falls through to the provider.

## Configuration

```yaml
runtime:
  cache:
    response_cache:
      enabled: true
      ttl: 24h
      max_entries_per_model: 1000     # per-namespace L2 index capacity (LRU)
      similarity_threshold: 0.92       # cosine threshold for an L2 hit
      semantic:
        enabled: true
        embedder: registry             # embed via a Polaris-configured model
        embedding_model: openai/text-embedding-3-small
        max_turns: 4                   # embed the last N user/assistant turns
        max_temperature: 0.3           # skip caching above this (unless opted in)
```

A caller can force a semantic lookup on a higher-temperature request with the
header `X-Polaris-Cache-Control: semantic`.

## How a hit works

1. Request-phase guardrails run first (so the cache stores post-redaction text).
2. The last `max_turns` turns are embedded via the configured model — routed
   through the provider layer **directly**, so the embed call never re-enters
   guardrails or the cache.
3. The query vector is scanned against the namespace's in-process cosine index.
   If the best match ≥ `similarity_threshold`, the stored response is returned
   with `X-Polaris-Cache: hit-semantic`.
4. On a miss, the response is generated, then embedded and stored.

The flat index is a contiguous normalized-vector matrix; a 5K × 1536-dim scan
completes in ~2.5 ms (benchmarked, `BenchmarkSemcacheLookup5K`).

## Isolation (security-grade)

Entries are namespaced by **model + settings + project + epoch**. Two requests
share a cache entry only if *every* field matches. A different project, model,
system prompt, or sampling setting can never return another's cached response —
`TestSemcacheIsolation` asserts each dimension in isolation.

The **epoch** folds in the embedder fingerprint and threshold, so changing the
embedding model or threshold transparently invalidates the old entries (they age
out by TTL).

## Degradation

The embedder is health-tracked. After repeated embed failures the cache enters
**exact-only** mode (`X-Polaris-Cache: degraded`, `polaris_semcache_degraded 1`)
and recovers automatically — a flaky embedding provider never fails requests, it
only forgoes semantic hits.

## Tuning the threshold

`similarity_threshold` trades recall for precision:

| Threshold | Behavior |
|---|---|
| 0.85 | Aggressive — more hits, higher risk of a loosely-related answer |
| 0.92 (default) | Balanced — near-paraphrases hit, distinct questions miss |
| 0.97+ | Conservative — essentially near-duplicates only |

Start at 0.92, watch `polaris_semcache_similarity` (the hit similarity histogram)
and `polaris_semcache_savings_usd`, and adjust.

## Operations

- **Metrics:** `polaris_cache_events_total{status,model}` (status includes
  `hit-semantic`), `polaris_semcache_similarity`, `polaris_semcache_savings_usd`,
  `polaris_semcache_degraded`.
- **Purge:** `POST /v1/admin/cache/purge` (admin) with an optional
  `{"model": "...", "project": "..."}` body; an empty body purges everything.

## Scope

Chat completions (unary). Streaming responses are not cached in v1. The L2 index
is per-replica (each gateway builds its own from traffic); L1 exact entries are
shared through the cache backend.
