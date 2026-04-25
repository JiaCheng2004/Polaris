# Phase 4E: Phase 4 Hardening and Acceptance

**Status:** In progress

## Summary

This milestone closes the remaining Phase 4 parity, validation, and release-readiness gaps without expanding the public modality surface beyond the Blueprint.

Validation record date: `2026-04-20`

## Acceptance Audit

| Area | Status | Evidence |
|---|---|---|
| Video modality | Pass | Seedance, Sora, and Veo adapters are implemented with submit, status, cancel/content handling, provider tests, gateway tests, and SDK coverage. |
| Full-duplex audio sessions | Pass | `POST /v1/audio/sessions` plus `GET /v1/audio/sessions/:id/ws` are live with OpenAI and ByteDance cascaded execution; covered by `internal/gateway/audio_cache_test.go`, provider audio helpers, and `pkg/client/audio_test.go`. |
| Response caching | Pass | Non-streaming chat, embeddings, images, TTS, and STT are cache-aware and emit `X-Polaris-Cache`; covered by `internal/gateway/audio_cache_test.go` and cache handler tests. |
| Usage parity | Pass | `GET /v1/usage` now accepts `modality=audio`; covered by the audio lifecycle usage test plus SQLite/Postgres usage-filter coverage. |
| Provider error normalization | Pass | Provider HTTP and transport failures now pass through one shared Polaris translator, preserving a consistent error envelope and stable subcodes such as `quota_exceeded`, `provider_auth_failed`, and `provider_timeout`. |
| Close-out validation harness | Pass | `config/polaris.live-smoke.yaml`, `tests/e2e/live_smoke_test.go`, and `make live-smoke` provide a committed live-smoke path for the shipped provider matrix. |
| Release surfaces | Pass | `CHANGELOG.md`, `docs/LOAD_TESTING.md`, and the tag-driven `.github/workflows/release.yml` now form the release-facing close-out surface. |

## Quality Gates

The Phase 4 close-out local gate is:

- `make release-check`

This expands to:

- `gofmt -l .`
- `go test -race ./...`
- `make build`
- `make stack-validate STACK=local`
- `make stack-validate STACK=prod`
- `make stack-validate STACK=dev`
- `make docker-build`

## Live Validation Gate

The release gate for `v2.0.0` is:

- `POLARIS_LIVE_SMOKE=1 POLARIS_LIVE_SMOKE_STRICT=1 make live-smoke`

The committed live matrix covers one representative path for every shipped provider surface:

- OpenAI: chat, embeddings, image, TTS, STT, video, audio sessions
- Anthropic: chat
- Google: chat, embeddings, image
- Google Vertex: video
- DeepSeek: chat
- ByteDance: chat, image, TTS, STT, video, audio sessions
- xAI: chat
- Qwen: chat, image
- Ollama: chat

Missing credentials, inactive provider services, or unavailable local Ollama models are release blockers in strict mode.

## Runtime Notes

- Phase 4 runtime features are implemented in code.
- Release close-out remains blocked until the strict live provider matrix is green.
- `cost_usd` remains estimate-only and defaults to `0` for unsupported or intentionally unpriced models.
- `context_window` remains best-effort model metadata. Polaris does not hard-enforce provider token windows and defers final token-limit rejection to the upstream provider.
- No new modality is introduced here. Music and future provider expansion remain later work.
- Consumer-chat or coding subscriptions are not treated as Polaris credentials; only official API auth models are in scope.

## Exit Criteria Result

Phase 4 can be called complete only when all of the following are true:

- the Blueprint Phase 4 surface is implemented and documented truthfully
- repo-local validation is green
- the strict live provider smoke matrix is green
- the load-validation checklist in `docs/LOAD_TESTING.md` is recorded as complete
- the `v2.0.0` tag is cut against that validated state

At the time of this record:

- repo-local close-out work is implemented
- release validation mechanics are committed
- final `v2.0.0` release completion is still pending the strict live matrix and release tag
