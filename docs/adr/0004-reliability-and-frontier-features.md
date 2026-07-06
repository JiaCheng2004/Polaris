# 4. Reliability primitives + four frontier features

Status: Accepted

## Context

By 2026 an AI gateway is judged on more than fan-out. Multi-hour provider
outages made adaptive failover table stakes; enterprises need guardrails with
audit evidence; cost pressure makes semantic caching material; and the MCP
ecosystem is moving to a stateless, streamable-HTTP, OAuth-2.1 profile that a
stateless gateway is uniquely placed to serve.

## Decision

Ship a reliability layer — circuit breaking, jittered retries with `Retry-After`,
load shedding, retry budgets, hedging, and idempotency keys — plus four fully
finished frontier features:

1. **Adaptive routing** — static/weighted/least-latency/cost-optimized/adaptive
   strategies with health-aware demotion.
2. **Guardrails** — PII/secrets/prompt-injection detection with
   observe/redact/block and streaming redaction.
3. **Semantic cache** — exact-hash L1 + embedding L2 with per-project isolation.
4. **MCP-native gateway** — stateless streamable-HTTP server + upstream proxy
   with OAuth 2.1.

Every one ships **off by default** and wire-neutral, with an explicit,
documented v1 scope boundary (guardrails are text-only; MCP is tools-only)
rather than half-built stubs.

## Consequences

- Default behavior is unchanged and golden-clean; features engage only under
  real failure or explicit config, so they are safe to ship before they are
  switched on.
- Reliability state is process-local and survives config hot-reload by
  construction (it lives outside the config snapshot).
- Scope boundaries are decisions with tests, not TODOs — each feature is
  complete within its stated boundary.
