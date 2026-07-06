# 2. Strict, lint-enforced package layering

Status: Accepted

## Context

A multi-provider, multi-modality gateway accretes coupling quickly: provider
adapters reach for gateway helpers, handlers import provider internals, and the
dependency graph becomes a cycle-ridden mess that resists testing and change.

## Decision

Dependencies flow one way:

```
modality ← apierror / obs / transport ← provider/* ← reliability / routing /
guardrails / semcache / mcp ← gateway ← cmd
```

Provider adapters and tooling **never** import `internal/gateway`. Shared error
types and observability live in neutral `internal/apierror` and `internal/obs`
packages; all outbound HTTP goes through one `internal/transport` core. The rule
is enforced in CI by `make check-layering` (a hard grep gate) plus golangci-lint
`depguard`, not left to convention.

## Consequences

- Providers are testable in isolation with `httptest`; they cannot smuggle in
  gateway concerns.
- One transport core means retries, jitter, `Retry-After`, and SSE decoding are
  implemented and fixed once, not per provider.
- Adding a provider is mechanical and confined to `internal/provider/<name>/`.
- The cost is up-front indirection (neutral packages, adapter kits), paid back
  by a graph that stays acyclic as the surface grows.
