# Contributing to Polaris

## Start Here

Read these files before making non-trivial changes:

1. `BLUEPRINT.md`
2. `CLAUDE.md`
3. `docs/API_REFERENCE.md` and `spec/openapi/polaris.v1.yaml` if your work touches HTTP behavior

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
- `pkg/client/`: public Go SDK for implemented Polaris endpoints

## PR Expectations

- Keep PRs narrow.
- If you change an endpoint, update `docs/API_REFERENCE.md`, `spec/openapi/polaris.v1.yaml`, and relevant contract fixtures in the same PR.
- If you change config schema, update `docs/CONFIGURATION.md`, `schema/polaris.config.schema.json`, and `schema/cue/polaris.config.cue`.
- If you add or change a provider, update `docs/PROVIDERS.md`.
- If you change the SDK surface, update `README.md`.
- Prefer one provider per PR.

## Testing Expectations

- Use `go test -race ./...` as the default test baseline.
- Use `make config-check` after config loader, config schema, provider snippet, routing snippet, or model catalog changes.
- Use `make contract-check` after endpoint, route, error envelope, or stable response-shape changes.
- Use `make release-check` for the repo-local close-out gate.
- Use `make stack-validate STACK=<name>` for CI-safe Compose validation; reserve `make stack-config STACK=<name>` for local rendered-config debugging.
- Use `make live-smoke` for the env-gated real-provider matrix when working on shipped provider paths.
- Use `make load-check` before release when provider quota is available.
- Adapter tests should use `httptest.NewServer`.
- Do not hit real provider APIs in tests.
- Store changes should be covered by integration tests.

## Implementation Phasing

The early blueprint phases are implemented. New work should follow `BLUEPRINT.md` §16: keep `/v1` stable, add provider variants only through the model matrix when a real adapter exists, and prioritize capability completion, routing quality, validation proof, and operational hardening over generic provider expansion.
