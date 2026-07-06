# Polaris Architecture

Polaris is a stateless, config-driven, multi-modality AI gateway written in Go. It exposes one stable `/v1` gateway surface across chat, embeddings, images, video, voice, audio sessions, music, model routing, usage tracking, and control-plane operations.

## Principles

- Keep the gateway stateless; persist projects, keys, policies, budgets, usage, and async job state through configured stores.
- Keep public APIs compatibility-first and stable under `/v1`.
- Preserve exact provider model identity internally while offering aliases, selectors, and model-family routing for callers.
- Keep provider credentials operator-managed through environment-backed config values.
- Keep provider adapters isolated from HTTP handlers.
- Keep handlers focused on HTTP translation and shared error envelopes.
- Keep runtime behavior config-driven through provider snippets, routing snippets, and the embedded model catalog.
- Keep tools and MCP access explicitly permissioned.

## Runtime Layers

```text
cmd/polaris/              process entrypoint
internal/config/          config loading, v2 imports, validation, hot reload
internal/modality/        shared request/response contracts
internal/provider/        provider adapters, catalog, registry, routing
internal/provider/common/ shared auth, transport, retry, conversion, contract helpers
internal/gateway/         Gin server, routes, middleware, handlers
internal/store/           store interfaces plus SQLite/PostgreSQL/cache implementations
pkg/client/               public Go SDK
config/                   local, reference, provider, routing, and smoke configs
schema/                   JSON Schema and CUE config contracts
spec/openapi/             machine-readable public HTTP contract
tests/                    contract, integration, e2e, live-smoke, and load validation
```

## Dependency Direction

Packages depend inward only; the direction is lint-enforced (`make check-layering`
+ depguard), so a provider or tooling package can never import the gateway:

```text
modality  ←  apierror / obs / transport  ←  provider/*  ←  reliability / routing /
             guardrails / semcache / mcp  ←  gateway  ←  cmd
```

New cross-cutting packages (reliability, guardrails, semcache, mcp) define their
own small sink interfaces (metrics, stores, tool sources); the gateway's concrete
types satisfy them structurally. This keeps those packages below the gateway
layer while still reporting through the shared observability spine.

## Request Lifecycle

A unary chat request flows through a stable pipeline:

1. **Middleware** (fixed order): recovery → request ID → tracing → runtime holder
   → body limit → CORS → logging → metrics → **auth** → **rate limit** → **budget**
   → usage. Auth resolves the API key / virtual key / signed headers into an
   `AuthContext` (allowed models, scopes, project).
2. **Bind + validate** the request against the modality contract.
3. **Guardrails (request phase)** — if a policy matches, run detectors and
   observe / redact / block *before* routing or caching.
4. **Semantic cache lookup** — on an eligible request, an exact then embedding
   lookup can short-circuit with a cached response.
5. **Routing** — the registry resolves the model (alias/selector/family), and the
   router orders candidates by the matched strategy (static, weighted,
   least-latency, cost, adaptive), demoting open-breaker targets.
6. **Execution with reliability** — each attempt passes breaker/shed admission,
   a per-attempt timeout, and health reporting; failover and hedging follow the
   policy. Streaming failover is only allowed before the first byte is written.
7. **Guardrails (response phase)** + **cache store** — the response is screened
   and (if enabled) cached post-redaction.
8. **Usage** is pushed onto a buffered channel and written asynchronously; the
   response path never blocks on the store.

Errors at any stage return the OpenAI-compatible envelope; streaming errors
terminate with an error frame + `[DONE]`.

## Frontier Subsystems

- `internal/reliability` — process-lifetime health (EWMA + sliding error window),
  circuit breakers, load shedding, retry budgets, hedging, and idempotency.
- `internal/routing` — compiled route policies and ordering strategies over the
  registry's candidate expansion.
- `internal/guardrails` — a detector engine (PII/secrets/prompt-injection/content
  + webhook/LLM-judge) with a streaming hold-back redactor.
- `internal/semcache` — exact + embedding response cache, namespaced for
  security-grade isolation, degrading to exact-only when the embedder is unhealthy.
- `internal/mcp` — a stateless streamable-HTTP MCP server, upstream client,
  aggregation, and OAuth 2.1 resource server.

## Policy vs State (Hot Reload)

The hot-reload invariant separates two kinds of data:

- **Policy** is a pure function of config — compiled route/guardrail policies,
  the registry, static keys, pricing. It lives in a `runtime.Snapshot` that is
  rebuilt and **atomically swapped** on reload.
- **State** accumulates at runtime — breaker/health scores, in-flight gauges,
  retry-budget buckets, semantic-cache indexes, the idempotency table, and the WS
  drain registry. It lives in process-lifetime managers constructed in
  `cmd/polaris/main.go` and **survives snapshot swaps by construction**.

A reload therefore builds a new snapshot, swaps it, then reconfigures the
process-lifetime managers — never dropping accumulated reliability state.

## Provider Pattern

Each provider owns its implementation under `internal/provider/<name>/` and registers through `internal/provider/registry_<name>.go`. New provider work must include a real adapter, catalog metadata, config snippet, docs, and tests. Placeholder provider directories are not part of the architecture.

Provider adapters implement only the relevant `internal/modality` interfaces. The registry resolves aliases, selectors, model families, fallback rules, and modality/capability checks before handlers call adapters.

## Gateway Pattern

Routes are grouped by domain and registered through `internal/gateway/routes*.go`. Middleware order is stable: recovery, request ID, tracing, runtime holder, body limits, CORS, logging, metrics, auth, rate limiting, budget enforcement, and usage logging.

Every error response uses the shared OpenAI-compatible error envelope. Endpoint behavior must stay aligned with `docs/API_REFERENCE.md`, `spec/openapi/polaris.v1.yaml`, and contract fixtures.

## Configuration Pattern

Config version `2` supports root YAML files with ordered imports. Provider configs live in `config/providers/`, routing configs live in `config/routing/`, and schema contracts live in `schema/`.

Provider credentials must be referenced through environment variables. Static and admin gateway keys must be stored as `sha256:` hashes.

## Validation Gates

- `make config-check` validates config loading, provider snippets, routing snippets, and model verification.
- `make contract-check` validates registered routes against OpenAPI and golden response fixtures.
- `make security-check` runs pinned gosec with an exact audited allowlist.
- `make test` runs the race-enabled Go test suite.
- `make release-check` is the full repo-local release gate.
- `make live-smoke` is env-gated proof for real provider access when credentials, quota, billing, and plan access are available.
