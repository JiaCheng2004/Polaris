# Polaris

Polaris is a stateless, config-driven, multi-modality AI gateway written in Go. It sits between applications and external AI providers and exposes one unified gateway surface for chat, images, video, voice, embeddings, and future full-duplex audio.

The project is being rebuilt as Polaris v2. The old Python monolith and Discord-bot infrastructure have been intentionally removed. This repository now tracks the Go gateway architecture defined in [BLUEPRINT.md](./BLUEPRINT.md).

## Status

- Architecture: finalized
- Repository bootstrap: initialized
- Current implementation phase: Phase 1
- Implemented code: placeholder scaffold only
- Source of truth: `BLUEPRINT.md`

That means the repo now reflects the target monorepo shape, deployment topology, config surface, and documentation set, but the actual gateway behavior still needs to be implemented phase by phase.

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

Some directories currently contain minimal placeholders so the repo shape is committed early without pretending the implementation already exists.

## Getting Started

### Local scaffold verification

```bash
make build
make test
```

At this stage, that validates the repo bootstrap and minimal package structure. It does not mean the Polaris gateway is feature-complete yet.

### Local single-binary development

Use the local config:

```bash
./bin/polaris --config ./config/polaris.yaml
```

`config/polaris.yaml` is intentionally development-oriented:

- SQLite store
- in-memory cache
- permissive `auth.mode: none`
- Phase 1 chat-provider references

### Docker and Compose

Deployment assets live under [`deployments/`](./deployments).

- [`deployments/Dockerfile`](./deployments/Dockerfile): target container image
- [`deployments/docker-compose.yml`](./deployments/docker-compose.yml): production-shaped placeholder stack with Polaris, PostgreSQL, and Redis
- [`deployments/docker-compose.dev.yml`](./deployments/docker-compose.dev.yml): development stack with Prometheus, Grafana, and pgAdmin added

These files are aligned to the target architecture and should be treated as bootstrap placeholders until the gateway implementation is further along.

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
