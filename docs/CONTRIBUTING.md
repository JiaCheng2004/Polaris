# Contributing to Polaris

## Start Here

Read these files before making non-trivial changes:

1. `BLUEPRINT.md`
2. `CLAUDE.md`
3. `docs/API_REFERENCE.md` if your work touches HTTP behavior

The blueprint is the source of truth. If a local file disagrees with it, the blueprint wins.

## Ground Rules

- Keep the project boring and dependency-light.
- Do not add dependencies outside `BLUEPRINT.md` §3 without approval.
- Providers never import handlers.
- Handlers never import provider packages directly.
- Middleware stays generic and cross-cutting.

## Repo Boundaries

- `internal/modality/`: interface and shared request/response contracts
- `internal/provider/<name>/`: one provider per PR
- `internal/gateway/handler/`: HTTP translation layer
- `internal/gateway/middleware/`: auth, rate limiting, logging, recovery
- `internal/store/`: database and cache abstractions
- `pkg/client/`: public Go SDK, deferred until Phase 3

## PR Expectations

- Keep PRs narrow.
- If you change an endpoint, update `docs/API_REFERENCE.md` in the same PR.
- If you change config schema, update `docs/CONFIGURATION.md`.
- If you add or change a provider, update `docs/PROVIDERS.md`.
- Prefer one provider per PR.

## Testing Expectations

- Use `go test -race ./...` as the default test baseline.
- Adapter tests should use `httptest.NewServer`.
- Do not hit real provider APIs in tests.
- Store changes should be covered by integration tests.

## Implementation Phasing

Work should follow the blueprint phases:

1. Foundation
2. Chat ecosystem
3. Image + voice + embeddings
4. Video + audio + polish

Do not skip ahead with deep implementation work that depends on unfinished lower-phase foundations.
