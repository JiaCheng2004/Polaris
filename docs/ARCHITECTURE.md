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
