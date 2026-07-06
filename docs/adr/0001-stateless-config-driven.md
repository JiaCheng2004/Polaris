# 1. Stateless, config-driven gateway

Status: Accepted

## Context

Polaris sits on the request path between applications and AI providers. A
gateway there must scale horizontally, survive restarts and rolling deploys
without losing correctness, and be operable by configuration rather than code
changes. Per-instance mutable state (sessions, sticky routing tables, in-memory
auth) would defeat all three.

## Decision

The gateway holds **no durable per-instance state**. All persistent data
(API-key hashes, projects, policies, usage) lives in an external `Store`
(SQLite or PostgreSQL); ephemeral coordination (rate-limit windows, cache) lives
in Redis when configured. Behavior is defined entirely by a YAML config with
`${ENV}` references, loaded into an immutable snapshot that is swapped
atomically on hot-reload. Sessions for full-duplex audio are stateless HMAC
tokens, not server memory.

## Consequences

- Any instance can serve any request; scale out is just more replicas behind a
  load balancer. Deploys are rolling with no session affinity.
- Config is the single operator surface; a reload never drops in-flight traffic
  (the snapshot swap is a pointer swap, and process-lifetime managers survive
  it).
- The cost is discipline: no feature may introduce per-instance state that
  changes request outcomes. Reliability state (breakers, health) is explicitly
  process-local and best-effort, never a correctness dependency.
