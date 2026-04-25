# Polaris

Polaris is a stateless, config-driven, multi-modality AI gateway written in Go. It sits between applications and external AI providers and exposes one unified gateway surface for chat, images, video, voice, embeddings, full-duplex audio sessions, and music.

The project is being rebuilt as Polaris v2. The old Python monolith and Discord-bot infrastructure have been intentionally removed. This repository now tracks the Go gateway architecture defined in [BLUEPRINT.md](./BLUEPRINT.md).

## Status

- Architecture: finalized
- Repository bootstrap: complete
- Current implementation phase: release-readiness consolidation and operational hardening
- Implemented code: Phase 1 foundation, full Phase 2, full Phase 3, the full Phase 4 runtime surface, Phase 5A music, and the post-Phase-5 provider-family/runtime hardening wave
- Current platform foundation work: virtual-key control plane, MCP/tool runtime, OTLP tracing, and the next conversation surfaces (`/v1/responses`, `/v1/messages`, `/v1/tokens/count`) are implemented in the runtime and docs, including SSE streaming for `responses` and `messages` plus native token counting for Anthropic and Gemini chat models
- Current ByteDance speech coverage: TTS 2.0, file STT 2.0, streaming STT 2.0, realtime audio sessions, simultaneous interpretation 2.0, machine translation, voice catalog and voice assets, audio notes, and podcast generation are implemented and live-validated in the provider smoke matrix
- Current provider-family hardening: the shared OpenAI-compatible adapter base is now in the runtime, the OpenAI catalog includes `gpt-5.5` and `gpt-image-2`, the chat-first families OpenRouter, Together, Groq, Fireworks, Featherless, Moonshot, GLM, Mistral, and NVIDIA are wired through the same provider-common path, Amazon Bedrock is in the runtime through native Converse/ConverseStream plus Titan embedding adapters, NVIDIA now includes embeddings on the same official `/v1/embeddings` path, and Replicate is in the runtime through a native Predictions-based async video adapter plus an embedded YAML provider model matrix
- Release status: `v2.1.0` close-out is blocked on worktree consolidation and the final open-source readiness pass, not on more provider expansion. The repo-local gate, strict live-smoke matrix, reduced local load check, and targeted OpenAI Realtime concurrency check have passed in the current close-out record. Production Postgres/Redis load testing remains optional operator validation for service deployments, not a default open-source release blocker. Live-smoke coverage is matrix-driven: `strict` models are release-blocking, `opt_in` models run only when explicitly enabled, and `skipped` models stay outside the default matrix.
- Source of truth: `BLUEPRINT.md`

The repository now contains the full Phase 1 foundation from the blueprint: bootable server wiring, config loading and validation, SQLite storage, in-memory rate limiting, static auth, model registry, OpenAI and Anthropic chat adapters, usage logging, and the Phase 1 endpoints.

## What Polaris Is

- A unified gateway for multiple AI modalities
- OpenAI-compatible where a standard already exists
- Config-driven for providers, models, aliases, and failover
- Capability-driven selector aliases for intent-based model routing
- Family-aware model routing where canonical family IDs resolve to provider variants without rewriting exact provider-model requests
- Optional request-level routing hints across model-taking endpoints, including multipart endpoints through a JSON `routing` form field
- Built-in configured-model verification reports via `make verify-models`
- Operator-managed virtual keys over provider-owned credentials
- Bring-your-own-auth mode for platforms that already own OAuth, SMS OTP, SSO, sessions, and users
- Policy-gated tool and MCP brokering
- Stateless at runtime, with persistence delegated to external stores
- Designed to run as a single binary with SQLite or scale out with PostgreSQL and Redis

## What Polaris Is Not

- Not an agent framework
- Not a prompt orchestration system
- Not a model host
- Not a chat UI
- Not a catch-all replacement for application business logic

Consumer apps should keep prompts, RAG, search, user management, and product logic outside Polaris. Polaris owns provider access, routing, auth, rate limiting, usage tracking, and operational gateway concerns.

## Roadmap by Phase

### Phase 1: Foundation

- Config loader and validation
- SQLite store and in-memory cache
- Static auth and rate limiting
- Core middleware and gateway routing
- Chat modality only
- OpenAI and Anthropic chat adapters
- Health, models, and usage endpoints
- Docker image and baseline tests

### Phase 2: Chat Ecosystem

- DeepSeek, Google, xAI, Qwen, ByteDance, Ollama, OpenRouter, Together, Groq, Fireworks, Featherless, Moonshot, GLM, Mistral, Bedrock, and NVIDIA chat adapters, plus Replicate async video adapters
- PostgreSQL store and Redis cache
- Failover, aliases, hot reload, metrics, and Grafana

Phase 2 completed scope:

- implemented: PostgreSQL store, Redis cache
- implemented: DeepSeek chat, xAI chat, Ollama chat
- implemented: Google Gemini chat, Qwen chat
- implemented: shared OpenAI-compatible provider base plus OpenRouter, Together, Groq, Fireworks, Featherless, Moonshot, GLM, Mistral, and NVIDIA chat-provider families
- implemented: Amazon Bedrock native `Converse` / `ConverseStream` chat-provider family
- implemented: Replicate native Predictions-based async video-provider family
- implemented: capability-driven routing selectors plus family-aware model routing in config/registry resolution and `/v1/models?include_aliases=true`
- implemented: ByteDance Doubao chat
- implemented: runtime failover with `X-Polaris-Fallback`
- implemented: hot reload via `SIGHUP` and file watch
- implemented: Prometheus metrics and Grafana dashboard alignment
- implemented: docs, stack commands, and acceptance checks reconciled to the shipped runtime

### Phase 3: Image + Voice + Embeddings

- Image adapters for OpenAI, Google, ByteDance, and Qwen
- Voice adapters for OpenAI and ByteDance
- Embeddings for OpenAI and Google
- Multi-user auth
- Public Go SDK in `pkg/client`

Phase 3 completed scope:

- implemented in Phase 3: `auth.mode: multi-user`
- implemented and still shipped as compatibility: `POST /v1/keys`, `GET /v1/keys`, and `DELETE /v1/keys/:id`
- current preferred control-plane path: `auth.mode: virtual_keys` plus `/v1/projects`, `/v1/virtual_keys`, `/v1/policies`, `/v1/budgets`, `/v1/tools`, `/v1/toolsets`, and `/v1/mcp/bindings`
- implemented: real embed, image, and voice modality contracts
- implemented: registry lookup and model metadata for embed, image, and voice models
- implemented: `POST /v1/embeddings` end to end for OpenAI, Google, Amazon Bedrock Titan embeddings, and NVIDIA embeddings
- implemented: `POST /v1/images/generations` and `POST /v1/images/edits` end to end for OpenAI, Google, ByteDance, and Qwen
- implemented: `POST /v1/audio/speech` end to end for OpenAI and ByteDance TTS
- implemented: `POST /v1/audio/transcriptions` end to end for OpenAI and ByteDance STT
- implemented: mixed-modality usage reporting and known-cost estimation without schema changes
- implemented: public Go SDK in `pkg/client` for chat, embeddings, images, voice, models, usage, and admin keys
- completed: Phase 3 close-out acceptance in `spec/phase_3_image_voice_embeddings/3J_phase_3_hardening_and_acceptance.md`

### Phase 4: Video + Audio + Polish

- implemented: Seedance video Phase 4A async job flow plus Phase 4B request parity for `last_frame`, `reference_videos`, and synced input `audio`
- implemented: Phase 4C video hardening with Polaris-owned `GET /v1/video/generations/:id/content`
- implemented: OpenAI Sora (`sora-2`, `sora-2-pro`) and Google Vertex Veo (`google-vertex`) video providers
- implemented: per-model video metadata in `/v1/models` via `allowed_durations`, `aspect_ratios`, and `cancelable`
- implemented: public Go SDK video helpers in `pkg/client/video.go`, including content download
- implemented: full-duplex audio sessions via `POST /v1/audio/sessions` and `GET /v1/audio/sessions/:id/ws`
- implemented: OpenAI native Realtime audio sessions and ByteDance native realtime audio sessions behind the shared `modality: audio` contract; explicit cascaded `audio_pipeline` compatibility remains available for older configurations
- implemented: response caching for non-streaming chat, embeddings, images, TTS, and STT with `X-Polaris-Cache`
- implemented: Phase 4 close-out validation assets in `config/polaris.live-smoke.yaml`, `tests/e2e/live_smoke_test.go`, `docs/LOAD_TESTING.md`, and `spec/phase_4_video_audio_polish/4E_phase_4_hardening_and_acceptance.md`

### Phase 5A: Music

- implemented: first-class `music` modality in the shared registry, model catalog, usage layer, and Go SDK
- implemented: unified music endpoints for generation, edits, stems, lyrics, plans, async job polling, cancellation, and content download
- implemented: Polaris-managed async music jobs backed by the configured cache
- implemented: MiniMax generation, cover-edit, and lyrics adapters as the `v2.1.0` release-blocking music path
- implemented in preview: ElevenLabs generation, streaming generation, composition plans, and stems adapters
- implemented: exact-match caching for synchronous music generation, edit, stems, lyrics, and plan calls
- next: release-readiness consolidation, worktree organization, optional ElevenLabs preview smoke, optional operator Postgres/Redis load validation, and the final `v2.1.0` release tag

## Repository Layout

The repo follows the target layout from `BLUEPRINT.md`:

```text
cmd/polaris/              entrypoint
internal/config/          config loading, validation, hot reload
internal/modality/        shared modality contracts
internal/provider/        provider adapters and registry
internal/provider/common/ reusable provider-family transport, auth, and SSE helpers
internal/gateway/         Gin server, handlers, middleware
internal/store/           store abstractions, cache, migrations
pkg/client/               public Go SDK
config/                   local and reference YAML config
deployments/              Docker, Compose, Grafana, Prometheus, pgAdmin
docs/                     API, configuration, provider, and contributor docs
scripts/                  helper entrypoints such as migration tooling
tests/                    integration, e2e, live-smoke, and load-check validation
```

Provider and modality directories should now be treated as shipped runtime areas or explicit preview work. Avoid adding placeholder provider trees without a concrete adapter, config entry, matrix entry, and validation path.

## Getting Started

### Developer commands

The stable command surface for this repo is the `Makefile`. Treat it as the Go-repo equivalent of `npm run ...`.

```bash
make dev
make build
make test
make release-check
make live-smoke
make lint
make stack-up STACK=local
make stack-logs STACK=local
make stack-down STACK=local
```

The underlying implementation may change in later phases, but these command names should remain the main developer entrypoints.

`make release-check` is the current repo-local release gate. `make live-smoke` runs the env-gated real-provider smoke matrix and becomes a release blocker when `POLARIS_LIVE_SMOKE_STRICT=1`. ElevenLabs music smoke is preview-only for `v2.1.0` and runs only when `POLARIS_LIVE_SMOKE_ELEVENLABS=1` is also set.

`make release-check` validates Compose files through the quiet `stack-validate` path so shared CI logs do not render environment-expanded service configuration. Use `make stack-config STACK=<name>` only when you intentionally need to inspect the fully rendered Compose config locally.

For container workflows:

- `STACK=local`: current Phase 1 local runtime
- `STACK=prod`: production-shaped Compose stack
- `STACK=dev`: production-shaped stack plus Prometheus, Grafana, and pgAdmin

This keeps the developer command surface stable while letting the underlying stack evolve phase by phase.

### Release-candidate verification

```bash
make build
make test
make release-check
```

That is the default repo-local validation baseline for the `v2.1.0` close-out candidate. Release completion also records the env-gated strict live smoke matrix and reduced local load validation when credentials are available. Production Postgres/Redis load validation is optional operator proof for service deployments, not a default contributor requirement.

### Local single-binary development

Use the local config:

```bash
./bin/polaris --config ./config/polaris.yaml
```

`config/polaris.yaml` is intentionally development-oriented:

- SQLite store
- in-memory cache
- permissive `auth.mode: none`
- OpenAI and Anthropic Phase 1 chat-provider references

## Go SDK

`pkg/client` is the public Go SDK for the implemented Polaris surface. It wraps the live HTTP API and keeps SDK-owned request and response types.

Supported helpers:

- `CreateChatCompletion`, `StreamChatCompletion`
- `CreateResponse`, `StreamResponse`, `CreateMessage`, `StreamMessage`, `CountTokens`
- `CreateEmbedding`
- `GenerateImage`, `EditImage`
- `CreateMusicGeneration`, `StreamMusicGeneration`, `EditMusic`, `StreamMusicEdit`, `SeparateMusicStems`, `CreateMusicLyrics`, `CreateMusicPlan`, `GetMusicJob`, `GetMusicJobContent`, `CancelMusicJob`
- `CreateSpeech`, `CreateTranscription`
- `CreateAudioSession`, `DialAudioSession`
- `ListModels`, `GetUsage`
- `CreateKey`, `ListKeys`, `DeleteKey`
- `CreateProject`, `ListProjects`
- `CreateVirtualKey`, `ListVirtualKeys`, `DeleteVirtualKey`
- `CreatePolicy`, `ListPolicies`
- `CreateBudget`, `ListBudgets`
- `CreateTool`, `ListTools`
- `CreateToolset`, `ListToolsets`
- `CreateMCPBinding`, `ListMCPBindings`

Quick start:

```go
package main

import (
	"context"
	"log"
	"os"

	"github.com/JiaCheng2004/Polaris/pkg/client"
)

func main() {
	ctx := context.Background()

	sdk, err := client.New(
		"http://localhost:8080",
		client.WithAPIKey(os.Getenv("POLARIS_KEY")),
	)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := sdk.CreateChatCompletion(ctx, &client.ChatCompletionRequest{
		Model: "default-chat",
		Messages: []client.ChatMessage{
			{Role: "user", Content: client.NewTextContent("Say hello.")},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	if text := resp.Choices[0].Message.Content.Text; text != nil {
		log.Println(*text)
	}
}
```

`pkg/client/video.go` exposes the video helpers. `pkg/client/music.go` now exposes the music generation, edit, stems, lyrics, plan, streaming, and async job helpers.

### Docker and Compose

Deployment assets live under [`deployments/`](./deployments).

- [`deployments/Dockerfile`](./deployments/Dockerfile): target container image
- [`deployments/docker-compose.local.yml`](./deployments/docker-compose.local.yml): current Phase 1 local stack using `config/polaris.yaml` with SQLite and in-memory cache
- [`deployments/docker-compose.yml`](./deployments/docker-compose.yml): production-shaped stack with Polaris, PostgreSQL, and Redis
- [`deployments/docker-compose.dev.yml`](./deployments/docker-compose.dev.yml): development stack with Prometheus, Grafana, and pgAdmin added

For the current local runtime, use the dedicated wrapper script:

```bash
cp .env.example .env
./scripts/stack.sh up local
./scripts/stack.sh logs local
./scripts/stack.sh down local
```

Equivalent Make targets are available:

```bash
make stack-up STACK=local
make stack-logs STACK=local
make stack-down STACK=local
```

The production and dev Compose files are also addressable through the same interface:

```bash
make stack-validate STACK=prod
make stack-validate STACK=dev
make stack-config STACK=prod
make stack-config STACK=dev
```

`stack-validate` checks Compose syntax without printing the interpolated service configuration. `stack-config` renders the effective config and may include environment-derived values, so treat it as a local debugging command.

For local Compose usage, `.env.example` is the starting point for a developer `.env` file.

## Configuration

Two config files are committed on purpose:

- [`config/polaris.yaml`](./config/polaris.yaml): local-development default
- [`config/polaris.example.yaml`](./config/polaris.example.yaml): full reference config

Configuration precedence is:

1. CLI flags
2. Environment variables
3. YAML config
4. Built-in defaults

Secrets must always come from environment variables via `${VAR_NAME}` references. Do not commit plaintext provider keys, gateway secrets, or local `.env` files.

## Documentation Map

- [`BLUEPRINT.md`](./BLUEPRINT.md): architecture and implementation source of truth
- [`spec/phase_1_foundation/README.md`](./spec/phase_1_foundation/README.md): Phase 1 ABZ package
- [`spec/phase_2_chat_ecosystem/README.md`](./spec/phase_2_chat_ecosystem/README.md): Phase 2 ABZ package
- [`spec/phase_3_image_voice_embeddings/README.md`](./spec/phase_3_image_voice_embeddings/README.md): Phase 3 ABZ package
- [`CLAUDE.md`](./CLAUDE.md): repo-local agent workflow companion
- [`docs/API_REFERENCE.md`](./docs/API_REFERENCE.md): target HTTP contract
- [`docs/AUTHENTICATION.md`](./docs/AUTHENTICATION.md): local, static, external, virtual-key, and compatibility auth modes
- [`docs/CONFIGURATION.md`](./docs/CONFIGURATION.md): config guidance
- [`docs/PROVIDERS.md`](./docs/PROVIDERS.md): provider-specific auth and quirks
- [`docs/CONTRIBUTING.md`](./docs/CONTRIBUTING.md): contributor rules and phase boundaries

## Migration Note

Polaris v1 was a Python/FastAPI monolith coupled to Discord-bot infrastructure. Polaris v2 shares zero code with that stack.

What moved out of this repository:

- Discord bot logic
- PostgREST and related application APIs
- Lavalink and bot-side audio infrastructure
- Document parsing, search, vector storage, and other consumer concerns

What remains in scope for Polaris:

- provider API access
- model routing
- auth and rate limiting
- failover
- usage tracking
- gateway operations

## Contribution Rules

- Read `BLUEPRINT.md` before changing architecture, config, APIs, or providers.
- Do not add dependencies outside the approved stack.
- Keep provider work isolated to one provider per PR.
- Update `docs/API_REFERENCE.md` in the same PR as any endpoint change.
- Keep secrets out of the repo.

## License

Polaris remains licensed under AGPL-3.0. See [`LICENSE`](./LICENSE).
