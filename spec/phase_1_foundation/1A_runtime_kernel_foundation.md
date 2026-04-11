# Phase 1A: Runtime Kernel Foundation

**Status:** Implemented

## Summary

This milestone built the concrete runtime kernel that every later Phase 1 feature depends on. It established the server bootstrap, config system, storage substrate, cache substrate, registry metadata layer, and the generic middleware spine.

It intentionally stopped before real provider chat routing so the core contracts could settle first.

## Delivered Scope

- **Config and bootstrap**
  - Typed config structs for server, auth, store, cache, providers, routing, and observability
  - YAML load, `${VAR}` expansion, built-in defaults, validation, and runtime overrides
  - `cmd/polaris` as the composition root with graceful shutdown

- **Modality and contracts**
  - Real chat request and response contracts in `internal/modality/chat.go`
  - Concrete modality and capability enums in `internal/modality/capabilities.go`

- **Store and cache substrate**
  - Real store interfaces and persistence models
  - Working SQLite implementation with migrations
  - In-memory cache implementation
  - Async request-log writer with buffered batching and retry-once behavior

- **Gateway kernel**
  - OpenAI-style HTTP error envelope
  - Recovery, request ID, CORS, structured logging, auth, and rate limiting middleware
  - `GET /health`, `GET /ready`, and `GET /v1/models`
  - Registry-backed model metadata and alias resolution

- **Verification**
  - Kernel-level tests for config, registry metadata, SQLite store, and HTTP route basics
  - `go build ./cmd/polaris` and `go test -race ./...`

## Stable Interfaces Established By 1A

- config loading, validation, and runtime override behavior
- chat modality types shared by handlers and adapters
- store interface and persistence models
- cache interface for rate limiting and lightweight lookups
- registry model-metadata lookup and alias resolution
- common HTTP error serialization format

## Intentional Non-Goals

`1A` did not implement:

- provider clients or provider adapters
- adapter-aware registry lookup
- `POST /v1/chat/completions`
- usage middleware and `GET /v1/usage`
- PostgreSQL, Redis, multi-user auth, failover execution, or metrics

Those are deferred to `1B` and later phases.

## Handoff To 1B

The next step starts from this kernel and adds the first real product path:

- instantiate OpenAI and Anthropic clients from config
- register chat adapters in the registry
- serve chat completions over JSON and SSE
- capture and persist usage data
- expose usage reporting for the authenticated key
