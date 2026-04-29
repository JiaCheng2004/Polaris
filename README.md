<div align="center">

# Polaris

**A stateless, config-driven AI gateway for routing one application across many model providers and modalities.**

[![Go](https://img.shields.io/badge/Go-1.26.2-00ADD8?style=for-the-badge&logo=go)](https://go.dev/)
[![API](https://img.shields.io/badge/API-v1-2563EB?style=for-the-badge)](./docs/API_REFERENCE.md)
[![Config](https://img.shields.io/badge/Config-v2-16A34A?style=for-the-badge)](./docs/CONFIGURATION.md)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?style=for-the-badge&logo=docker&logoColor=white)](./deployments/Dockerfile)
[![License](https://img.shields.io/badge/License-AGPL--3.0-F97316?style=for-the-badge)](./LICENSE)

[Quick Start](#quick-start) · [API Surface](#api-surface) · [Providers](#providers) · [Configuration](#configuration) · [Documentation](#documentation)

[简体中文](./README.zh-CN.md)

</div>

---

## What Is Polaris?

Polaris is a Go gateway that sits between your application and upstream AI providers. Your app calls one stable Polaris API, while Polaris handles provider credentials, model routing, failover, authentication, rate limiting, usage logging, response caching, and operational safety.

It is designed for teams that want one self-hosted gateway for multiple model families and modalities without moving product logic, prompts, RAG, user sessions, or business workflows into the gateway.

### What Polaris Handles

| Area | What is implemented |
| --- | --- |
| Unified `/v1` API | Chat, responses, messages, embeddings, images, video, voice, audio sessions, transcription, translation, notes, podcasts, music, models, usage, keys, and control-plane resources. |
| Provider routing | Provider-native model IDs, aliases, selector aliases, family-aware routing, request-level routing hints, and configured fallback chains. |
| Authentication | Local no-auth mode, static bearer keys, external signed-header auth, virtual keys, and legacy multi-user compatibility. |
| Control plane | Projects, virtual keys, policies, budgets, tools, toolsets, and MCP bindings. |
| Storage | SQLite for local use, PostgreSQL for production-shaped deployments, memory cache, and Redis cache. |
| Operations | Prometheus metrics, structured logs, optional OpenTelemetry tracing, request IDs, body limits, CORS controls, Docker, Compose, and release validation commands. |
| Go SDK | `pkg/client` wraps the shipped HTTP endpoints for Go applications. |

### What Polaris Does Not Handle

Polaris is not a workflow orchestrator, prompt framework, RAG engine, model host, vector database, chat UI, or application-auth provider. Keep user login, Google OAuth, SMS OTP, SSO, product permissions, prompts, retrieval, and business workflows in your application. Polaris should be the gateway layer underneath them.

## Project Status

The current codebase ships a broad multi-provider runtime with local validation gates. Real-provider proof depends on credentials, quota, billing, regional availability, and provider plan access.

Signed multi-architecture container images are published to GitHub Container Registry on every `main` push and on every `v*.*.*` tag. See [Container Image](./docs/CONFIGURATION.md#container-image) for tag policy, supported platforms, and how to verify the cosign signature and SLSA build provenance.

Use this rule of thumb:

- `make release-check` proves the repository builds, tests, contracts, configs, security checks, and Docker image locally.
- `make live-smoke` proves real upstream provider access only when the required environment variables and provider access are available.
- Missing provider credentials are not a local development blocker; they only block claims that a provider was live-smoked in your environment.

## Quick Start

### 1. Prerequisites

- Go `1.26.2`
- Git
- Docker Desktop or Docker Engine, only if you use Compose or Docker validation
- At least one provider credential for real model calls, unless you use a local provider such as Ollama

### 2. Clone And Build

```bash
git clone https://github.com/JiaCheng2004/Polaris.git
cd Polaris
make build
```

The binary is written to `./bin/polaris`. Confirm the build with `./bin/polaris --version`.

If you do not need to modify the source, pull a published image instead:

```bash
docker pull ghcr.io/jiacheng2004/polaris:edge      # rolling main
docker pull ghcr.io/jiacheng2004/polaris:vX.Y.Z    # immutable release
```

### 3. Run The Local Gateway

The default config is [`config/polaris.yaml`](./config/polaris.yaml). It binds to `127.0.0.1:8080`, uses SQLite, uses the in-memory cache, and sets `runtime.auth.mode: none` for local development.

```bash
export OPENAI_API_KEY=<your-openai-key>
make run
```

In another terminal:

```bash
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/v1/models
```

Call the OpenAI-compatible chat endpoint:

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "default-chat",
    "messages": [
      {"role": "user", "content": "Explain Polaris in one sentence."}
    ]
  }'
```

Because the local config uses `auth.mode: none`, the request above does not need a Polaris API key. Do not expose that config publicly.

### 4. Run With Docker Compose

```bash
cp .env.example .env
make stack-up STACK=local
make stack-logs STACK=local
make stack-down STACK=local
```

Available stacks:

| Stack | Command | Purpose |
| --- | --- | --- |
| `local` | `make stack-up STACK=local` | Single Polaris service with SQLite and memory cache. |
| `prod` | `make stack-up STACK=prod` | Production-shaped Polaris, PostgreSQL, and Redis stack. |
| `dev` | `make stack-up STACK=dev` | Production-shaped stack plus Prometheus, Grafana, and pgAdmin. |

Use `make stack-validate STACK=<local|prod|dev>` to validate Compose files without printing interpolated secrets.

### 5. Use A Local Ollama Model

Ollama is implemented as a native chat provider. Start Ollama first:

```bash
ollama serve
ollama pull llama3
```

Create a local-only config that imports [`config/providers/ollama.yaml`](./config/providers/ollama.yaml) and a routing alias:

```yaml
version: 2
imports:
  - ./providers/ollama.yaml
runtime:
  server:
    host: 127.0.0.1
    port: 8080
  auth:
    mode: none
  store:
    driver: sqlite
    dsn: ./polaris.db
  cache:
    driver: memory
routing:
  aliases:
    default-chat: ollama/llama3
```

Then run Polaris with that config:

```bash
go run ./cmd/polaris --config ./config/local.ollama.yaml
```

## API Surface

Polaris exposes a stable `/v1` gateway surface plus health, metrics, and MCP proxy routes.

| Category | Endpoints |
| --- | --- |
| Health and metrics | `GET /health`, `GET /ready`, `GET /metrics` |
| Chat and conversation | `POST /v1/chat/completions`, `POST /v1/responses`, `POST /v1/messages`, `POST /v1/tokens/count` |
| Embeddings and translation | `POST /v1/embeddings`, `POST /v1/translations` |
| Images | `POST /v1/images/generations`, `POST /v1/images/edits` |
| Video | `POST /v1/video/generations`, `GET /v1/video/generations/:id`, `GET /v1/video/generations/:id/content`, `DELETE /v1/video/generations/:id` |
| Voice and audio | `POST /v1/audio/speech`, `POST /v1/audio/transcriptions`, `POST /v1/audio/transcriptions/stream`, `GET /v1/audio/transcriptions/stream/:id/ws`, `POST /v1/audio/sessions`, `GET /v1/audio/sessions/:id/ws` |
| Interpretation, notes, podcasts | `POST /v1/audio/interpreting/sessions`, `GET /v1/audio/interpreting/sessions/:id/ws`, `POST /v1/audio/notes`, `GET /v1/audio/notes/:id`, `DELETE /v1/audio/notes/:id`, `POST /v1/audio/podcasts`, `GET /v1/audio/podcasts/:id`, `GET /v1/audio/podcasts/:id/content`, `DELETE /v1/audio/podcasts/:id` |
| Music | `POST /v1/music/generations`, `POST /v1/music/edits`, `POST /v1/music/stems`, `POST /v1/music/lyrics`, `POST /v1/music/plans`, `GET /v1/music/jobs/:id`, `GET /v1/music/jobs/:id/content`, `DELETE /v1/music/jobs/:id` |
| Voice resources | `GET /v1/voices`, `GET /v1/voices/:id`, `DELETE /v1/voices/:id`, `POST /v1/voices/:id/archive`, `POST /v1/voices/:id/unarchive`, `POST /v1/voices/clones`, `POST /v1/voices/designs`, `POST /v1/voices/:id/retrain`, `POST /v1/voices/:id/activate` |
| Models and usage | `GET /v1/models`, `GET /v1/usage` |
| Keys and control plane | `POST /v1/keys`, `GET /v1/keys`, `DELETE /v1/keys/:id`, `POST /v1/projects`, `GET /v1/projects`, `POST /v1/virtual_keys`, `GET /v1/virtual_keys`, `DELETE /v1/virtual_keys/:id`, `POST /v1/policies`, `GET /v1/policies`, `POST /v1/budgets`, `GET /v1/budgets`, `POST /v1/tools`, `GET /v1/tools`, `POST /v1/toolsets`, `GET /v1/toolsets`, `POST /v1/mcp/bindings`, `GET /v1/mcp/bindings` |
| MCP broker | `ANY /mcp/:binding_id`, `ANY /mcp/:binding_id/*path` |

Full request and response details live in [`docs/API_REFERENCE.md`](./docs/API_REFERENCE.md). The machine-readable OpenAPI contract is [`spec/openapi/polaris.v1.yaml`](./spec/openapi/polaris.v1.yaml).

## Providers

Provider adapters are isolated under [`internal/provider`](./internal/provider), configured through [`config/providers`](./config/providers), and registered through provider-owned `registry_<provider>.go` files.

| Provider | Current Polaris scope |
| --- | --- |
| OpenAI | Chat, responses, embeddings, images, voice, video, native realtime audio sessions. |
| Anthropic | Chat and messages-compatible conversation surface. |
| Google Gemini | Chat, embeddings, images. |
| Google Vertex | Veo video. |
| Amazon Bedrock | Native Converse chat and Titan embeddings. |
| ByteDance / Volcengine | Chat, images, video, TTS, STT, streaming STT, realtime audio, interpretation, translation, notes, podcasts, voice catalog, and voice assets. |
| Qwen / DashScope | Chat and images. |
| DeepSeek, xAI, OpenRouter, Together, Groq, Fireworks, Featherless, Moonshot, GLM, Mistral, NVIDIA | Chat-first adapters through native or OpenAI-compatible provider surfaces; NVIDIA also supports embeddings. |
| Replicate | Async video through Predictions. |
| MiniMax | Music generation, cover edit, and lyrics. |
| ElevenLabs | Preview music generation, streaming generation, plans, and stems. |
| Ollama | Local chat through native Ollama API. |

Provider-specific credential rules and limitations are documented in [`docs/PROVIDERS.md`](./docs/PROVIDERS.md).

## Model Routing

Polaris accepts three model naming styles:

| Style | Example | Behavior |
| --- | --- | --- |
| Provider model | `openai/gpt-4o` | Runs exactly that configured provider/model pair. |
| Alias | `default-chat` | Resolves through `routing.aliases`. |
| Family or selector | `gpt-5.5`, `tooling-chat` | Resolves deterministically using the embedded model catalog, provider availability, configured selectors, and request-level routing hints. |

Model metadata is embedded from [`internal/provider/catalog/models.yaml`](./internal/provider/catalog/models.yaml). Validate configured models and aliases with:

```bash
make verify-models
make verify-models-json
```

## Configuration

Polaris uses YAML `version: 2` configs with ordered imports.

| File or directory | Purpose |
| --- | --- |
| [`config/polaris.yaml`](./config/polaris.yaml) | Local development defaults. |
| [`config/polaris.example.yaml`](./config/polaris.example.yaml) | Full reference config for production-shaped deployments. |
| [`config/polaris.live-smoke.yaml`](./config/polaris.live-smoke.yaml) | Environment-driven real-provider smoke config. |
| [`config/providers`](./config/providers) | Provider credentials, transport defaults, model use lists, and provider-specific overrides. |
| [`config/routing`](./config/routing) | Aliases, selectors, and fallback rules. |
| [`schema/polaris.config.schema.json`](./schema/polaris.config.schema.json) | JSON Schema contract for tooling. |
| [`schema/cue/polaris.config.cue`](./schema/cue/polaris.config.cue) | Optional CUE validation contract. |

Configuration precedence:

1. CLI flags
2. Environment variables
3. YAML config and imported YAML snippets
4. Built-in defaults

Secrets should be referenced through environment variables such as `${OPENAI_API_KEY}`. Do not commit plaintext provider keys, gateway keys, admin keys, TLS material, or local `.env` files.

## Authentication Modes

| Mode | Use case |
| --- | --- |
| `none` | Local-only development. Never expose publicly. |
| `static` | A small private deployment with fixed bearer keys in config. |
| `external` | Your platform owns login, OAuth, SMS OTP, SSO, sessions, and users; Polaris verifies signed request claims. |
| `virtual_keys` | Polaris owns projects, virtual keys, policies, budgets, toolsets, MCP bindings, and audit records. |
| `multi-user` | Compatibility path for older database-backed API key rows. |

For most product integrations, start with `external` if your app already has users, or `virtual_keys` if Polaris should be the API-key boundary.

Detailed setup is in [`docs/AUTHENTICATION.md`](./docs/AUTHENTICATION.md).

## Go SDK

The public Go SDK lives in [`pkg/client`](./pkg/client).

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

SDK helpers cover chat, streaming chat, responses, messages, token counting, embeddings, images, voice, streaming transcription, realtime audio sessions, interpreting sessions, video, music, notes, podcasts, models, usage, keys, and control-plane resources.

## Validation

Use the Makefile as the stable developer command surface:

| Command | What it proves |
| --- | --- |
| `make build` | Builds `./bin/polaris`. |
| `make test` | Runs `go test -race ./...`. |
| `make lint` | Runs pinned `golangci-lint`. |
| `make security-check` | Runs pinned `gosec` with the exact audited allowlist. |
| `make config-check` | Validates config loading, imports, and model catalog wiring. |
| `make contract-check` | Validates registered routes, OpenAPI coverage, and golden fixtures. |
| `make release-check` | Runs the full repo-local release gate, including Docker build and Compose validation. |
| `make live-smoke` | Runs env-gated real-provider smoke tests when credentials and provider access are available. |

## Repository Layout

```text
cmd/polaris/              process entrypoint
internal/config/          config loading, validation, imports, and hot reload
internal/modality/        shared provider contracts
internal/provider/        provider adapters, catalog, registry, and routing
internal/gateway/         HTTP server, handlers, routes, and middleware
internal/store/           store interfaces, SQLite, PostgreSQL, memory cache, Redis
internal/tooling/         local tool registry
pkg/client/               public Go SDK
config/                   local, reference, provider, routing, and smoke configs
schema/                   JSON Schema and CUE config contracts
deployments/              Docker, Compose, Prometheus, Grafana, and pgAdmin assets
docs/                     human documentation
spec/openapi/             machine-readable HTTP contract
tests/                    contract, integration, e2e, smoke, and load validation
```

## Documentation

| Document | Purpose |
| --- | --- |
| [`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md) | Runtime architecture and maintainability rules. |
| [`docs/API_REFERENCE.md`](./docs/API_REFERENCE.md) | Human-readable HTTP API contract. |
| [`spec/openapi/polaris.v1.yaml`](./spec/openapi/polaris.v1.yaml) | Machine-readable OpenAPI contract. |
| [`docs/CONFIGURATION.md`](./docs/CONFIGURATION.md) | Config format, imports, auth, providers, and routing details. |
| [`docs/AUTHENTICATION.md`](./docs/AUTHENTICATION.md) | Auth mode selection and external signed-header integration. |
| [`docs/PROVIDERS.md`](./docs/PROVIDERS.md) | Provider-specific setup, behavior, and limitations. |
| [`docs/ADDING_PROVIDER.md`](./docs/ADDING_PROVIDER.md) | Checklist for adding provider adapters safely. |
| [`docs/INTEGRATION_RECIPES.md`](./docs/INTEGRATION_RECIPES.md) | Copy-paste integration patterns. |
| [`docs/LOAD_TESTING.md`](./docs/LOAD_TESTING.md) | Local load validation guidance. |
| [`docs/CONTRIBUTING.md`](./docs/CONTRIBUTING.md) | Contributor expectations. |

## Contributing

Keep changes narrow and contract-driven:

- Keep provider code isolated in `internal/provider/<name>/`.
- Update API docs, OpenAPI, and contract fixtures when endpoint behavior changes.
- Add provider tests with `httptest.NewServer`; unit tests must not call real provider APIs.
- Keep secrets out of Git.
- Run `make release-check` before release-oriented changes.

## License

Polaris is licensed under [AGPL-3.0](./LICENSE).

---

> **Documentation notice:** This README was last updated on 04/26/2026. Provider APIs, model availability, pricing, and platform access rules can change over time; if this document has not been maintained recently, verify operational details against the current codebase and official provider documentation before production use.
