# Phase 1 Foundation

This folder is the ABZ package for Polaris Phase 1.

- `A`: current state after the runtime-kernel work
- `B`: the concrete implementation steps needed to finish Phase 1
- `Z`: the fully completed Phase 1 foundation layer described by `BLUEPRINT.md` section 16

`BLUEPRINT.md` remains the source of truth. These files break Blueprint Phase 1 into executable subplans with explicit exit criteria.

## ABZ Map

### A: Current State

The runtime kernel exists and is already implemented:

- typed config loading, env expansion, defaults, and validation
- SQLite store and in-memory cache foundation
- auth support for `none` and `static`
- request ID, recovery, CORS, structured logging, and rate limiting middleware
- registry-backed `GET /health`, `GET /ready`, and `GET /v1/models`
- bootable server wiring and baseline kernel tests

This is the floor for the rest of Phase 1. It is necessary, but it is not the full Phase 1 end state.

### B: Implementation Steps

| Step | Name | Status | Purpose |
|---|---|---|---|
| `1A` | [Runtime Kernel Foundation](./1A_runtime_kernel_foundation.md) | Implemented | Build the bootable gateway kernel and the core store/cache/middleware substrate. |
| `1B` | [Complete the Chat Foundation](./1B_complete_chat_foundation.md) | Planned | Add the first real end-to-end product path: OpenAI and Anthropic chat plus usage reporting. |
| `1C` | [Phase 1 Hardening and Acceptance](./1C_phase_1_hardening_and_acceptance.md) | Planned | Close the remaining quality, documentation, and acceptance gaps so Phase 1 can be called complete. |

### Z: End Goal

Phase 1 is complete only when Polaris can do all of the following:

- boot from config without manual code changes
- enforce `none` and `static` auth correctly
- apply request rate limiting
- serve `GET /health`, `GET /ready`, `GET /v1/models`, `POST /v1/chat/completions`, and `GET /v1/usage`
- route chat requests to real OpenAI and Anthropic adapters
- support both non-streaming and SSE chat responses
- write usage asynchronously to SQLite without blocking the response path
- report usage for the authenticated key
- pass the Phase 1 unit, integration, and race-test baseline
- keep the docs and claimed repo status aligned with the actual implementation

## Phase 1 Boundaries

The following items are explicitly out of scope for this Phase 1 package:

- PostgreSQL store
- Redis cache
- failover execution across providers
- config hot reload
- Prometheus metrics and Grafana
- multi-user auth and key-management endpoints
- DeepSeek, Google, xAI, Qwen, ByteDance, and Ollama chat adapters
- image, voice, embeddings, video, and full-duplex audio
- response caching and cost-table expansion beyond what Phase 1 usage tracking needs

Those belong to later Blueprint phases and should not be pulled into this package to finish chat faster.

## Exit Rule

Do not mark Phase 1 done after `1B` lands. `1C` is the explicit close-out step that confirms the implementation, tests, docs, and repository status all match the Blueprint's Phase 1 definition.
