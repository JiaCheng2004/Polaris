# Phase 2 Chat Ecosystem

This folder is the ABZ package for Polaris Phase 2.

- `A`: the current state after Phase 1 completion
- `B`: the ordered implementation steps required to finish Phase 2
- `Z`: the fully completed Phase 2 chat ecosystem described by `BLUEPRINT.md` section 16

`BLUEPRINT.md` remains the source of truth. These files break Blueprint Phase 2 into concrete, decision-complete subplans with explicit dependencies and exit criteria.

## ABZ Map

### A: Current State

Phase 1 is complete:

- bootable server with typed config loading and validation
- SQLite store and in-memory cache
- auth support for `none` and `static`
- request ID, recovery, CORS, structured logging, rate limiting, and usage logging middleware
- OpenAI and Anthropic chat adapters
- `GET /health`, `GET /ready`, `GET /v1/models`, `POST /v1/chat/completions`, and `GET /v1/usage`
- stable local and container developer command surface through `make` and `scripts/stack.sh`

That is the baseline for Phase 2. Phase 2 expands the chat provider matrix, completes the distributed runtime path, and adds the operational systems that become meaningful once Polaris supports a broader provider set.

### B: Implementation Steps

| Step | Name | Status | Purpose |
|---|---|---|---|
| `2A` | [Distributed Runtime Substrate](./2A_distributed_runtime_substrate.md) | Implemented | Make PostgreSQL, Redis, and the production-shaped runtime path real. |
| `2B` | [OpenAI-Compatible Chat Provider Wave](./2B_openai_compatible_chat_provider_wave.md) | Implemented | Add the lower-friction chat providers: DeepSeek, xAI, and Ollama. |
| `2C` | [Google and Qwen Chat Provider Wave](./2C_google_and_qwen_chat_provider_wave.md) | Implemented | Add the medium-complexity translation providers. |
| `2D` | [ByteDance Chat Provider Wave](./2D_bytedance_chat_provider_wave.md) | Implemented | Add ByteDance Doubao chat with endpoint-aware runtime routing. |
| `2E` | [Runtime Routing and Failover](./2E_runtime_routing_and_failover.md) | Implemented | Execute configured chat failovers at runtime and surface fallback responses explicitly. |
| `2F` | [Hot Reload Runtime](./2F_hot_reload_runtime.md) | Implemented | Reload runtime config safely without restarting the HTTP server. |
| `2G` | [Observability and Dashboards](./2G_observability_and_dashboards.md) | Implemented | Export `/metrics`, instrument request/runtime behavior, and align Grafana assets. |
| `2H` | [Phase 2 Hardening and Acceptance](./2H_phase_2_hardening_and_acceptance.md) | Implemented | Close Phase 2 with docs, acceptance, and end-to-end quality gates. |

### Z: End Goal

Phase 2 is complete only when Polaris can do all of the following:

- serve chat through OpenAI, Anthropic, DeepSeek, Google, xAI, Qwen, ByteDance (Doubao), and Ollama
- run with PostgreSQL and Redis as real runtime backends, not placeholders
- keep SQLite and the in-memory cache as supported local defaults
- resolve aliases and execute configured failovers at runtime
- reload providers, models, aliases, fallbacks, and rate-limit settings without restarting the HTTP server
- expose Prometheus metrics and ship a Grafana dashboard aligned to the emitted metrics
- keep the stable command surface intact while `STACK=local`, `STACK=prod`, and `STACK=dev` all describe real runtime paths
- pass the Phase 2 provider, integration, observability, concurrency, and acceptance test matrix
- keep the code, docs, and claimed repo status aligned with the real implementation

## Phase 2 Boundaries

The following items are explicitly out of scope for this Phase 2 package:

- image generation adapters
- voice adapters
- embedding adapters
- video generation
- full-duplex audio
- multi-user auth and `/v1/keys`
- response caching
- Go client SDK work
- load testing and release packaging work from later phases

Those belong to Blueprint Phase 3 or Phase 4 and should not be folded into the Phase 2 chat ecosystem.

## Sequence Rule

The intended implementation order is `2A` through `2H`.

- `2A` is the baseline Phase 2 step and must land before the distributed production path is considered valid.
- provider-wave steps may overlap in development, but they should merge in the declared order so translation and registry complexity grows gradually.
- `2E`, `2F`, and `2G` depend on the expanded provider matrix being real.
- `2H` is the explicit close-out step and is now the recorded Phase 2 acceptance gate.
