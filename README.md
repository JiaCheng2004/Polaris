# Polaris

Polaris is a stateless, config-driven, multi-modality AI gateway written in Go. It sits between applications and external AI providers and exposes one unified gateway surface for chat, images, video, voice, embeddings, and future full-duplex audio.

The project is being rebuilt as Polaris v2. The old Python monolith and Discord-bot infrastructure have been intentionally removed. This repository now tracks the Go gateway architecture defined in [BLUEPRINT.md](./BLUEPRINT.md).

## Status

- Architecture: finalized
- Repository bootstrap: complete
- Current implementation phase: Phase 1 complete
- Implemented code: runtime kernel, OpenAI and Anthropic chat routing, usage logging, and Phase 1 HTTP surface
- Source of truth: `BLUEPRINT.md`

The repository now contains the full Phase 1 foundation from the blueprint: bootable server wiring, config loading and validation, SQLite storage, in-memory rate limiting, static auth, model registry, OpenAI and Anthropic chat adapters, usage logging, and the Phase 1 endpoints.

## What Polaris Is

- A unified gateway for multiple AI modalities
- OpenAI-compatible where a standard already exists
- Config-driven for providers, models, aliases, and failover
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

- DeepSeek, Google, xAI, Qwen, ByteDance, and Ollama chat adapters
- PostgreSQL store and Redis cache
- Failover, aliases, hot reload, metrics, and Grafana

### Phase 3: Image + Voice + Embeddings

- Image adapters for OpenAI, Google, ByteDance, and Qwen
- Voice adapters for OpenAI and ByteDance
- Embeddings for OpenAI and Google
- Multi-user auth
- Public Go SDK in `pkg/client`

### Phase 4: Video + Audio + Polish

- Seedance video support
- Full-duplex audio design surface
- Response caching
- Cost tables, load testing, and release polish

## Repository Layout

The repo follows the target layout from `BLUEPRINT.md`:

```text
cmd/polaris/              entrypoint
internal/config/          config loading, validation, hot reload
internal/modality/        shared modality contracts
internal/provider/        provider adapters and registry
internal/gateway/         Gin server, handlers, middleware
internal/store/           store abstractions, cache, migrations
pkg/client/               public Go SDK
config/                   local and reference YAML config
deployments/              Docker, Compose, Grafana, Prometheus, pgAdmin
docs/                     API, configuration, provider, and contributor docs
scripts/                  helper entrypoints such as migration tooling
tests/                    integration and e2e placeholders
```

Future-phase directories still contain placeholders where Phase 2+ provider or modality work has not started yet.

## Getting Started

### Developer commands

The stable command surface for this repo is the `Makefile`. Treat it as the Go-repo equivalent of `npm run ...`.

```bash
make dev
make build
make test
make lint
make stack-up STACK=local
make stack-logs STACK=local
make stack-down STACK=local
```

The underlying implementation may change in later phases, but these command names should remain the main developer entrypoints.

For container workflows:

- `STACK=local`: current Phase 1 local runtime
- `STACK=prod`: production-shaped Compose stack
- `STACK=dev`: production-shaped stack plus Prometheus, Grafana, and pgAdmin

This keeps the developer command surface stable while letting the underlying stack evolve phase by phase.

### Phase 1 verification

```bash
make build
make test
```

At this stage, that validates the implemented Phase 1 foundation. Later phases are still pending.

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

### Docker and Compose

Deployment assets live under [`deployments/`](./deployments).

- [`deployments/Dockerfile`](./deployments/Dockerfile): target container image
- [`deployments/docker-compose.local.yml`](./deployments/docker-compose.local.yml): current Phase 1 local stack using `config/polaris.yaml` with SQLite and in-memory cache
- [`deployments/docker-compose.yml`](./deployments/docker-compose.yml): production-shaped placeholder stack with Polaris, PostgreSQL, and Redis
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
make stack-config STACK=prod
make stack-config STACK=dev
```

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
- [`CLAUDE.md`](./CLAUDE.md): repo-local agent workflow companion
- [`docs/API_REFERENCE.md`](./docs/API_REFERENCE.md): target HTTP contract
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
