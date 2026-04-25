# Phase 2D: ByteDance Chat Provider Wave

**Status:** Implemented

## Summary

This milestone isolates the most operationally distinct Phase 2 chat provider: ByteDance (Volcengine / Doubao). It gets its own step because its ModelArk chat runtime depends on endpoint-aware routing rather than the simpler single-base-URL assumptions used by the other OpenAI-like providers.

This step is chat only. Seedance, Seedream, and ByteDance voice features remain later-phase work.

## Key Changes

- **Volcengine client**
  - Implement `internal/provider/bytedance/client.go`.
  - Use the provider config to resolve:
    - ARK API key
    - endpoint / base URL
    - timeout and retry policy
  - Keep auth and endpoint resolution encapsulated in the client layer, not in the adapter.

- **Doubao chat adapter**
  - Implement `internal/provider/bytedance/chat.go`.
  - Translate between the Polaris chat contract and the Doubao chat wire format.
  - Support JSON and streaming chat completions if the provider endpoint supports them in the configured mode.
  - Map provider-side failures into Polaris error types.

- **Registry integration**
  - Register configured ByteDance chat models as real chat adapters.
  - Enable the provider when an ARK API key is configured.
  - Keep non-chat ByteDance model blocks present in config but unimplemented in runtime behavior until later phases.

- **Docs**
  - Update `docs/PROVIDERS.md` with:
    - ARK bearer-token auth
    - endpoint expectations
    - ByteDance chat-specific quirks
    - explicit note that voice/video/image remain later-phase work

## Public Interfaces Added Or Changed

- no new HTTP endpoints
- `/v1/chat/completions` gains support for ByteDance Doubao chat models

## Test Plan

- client tests:
  - request-header construction
  - endpoint override behavior
  - auth failure behavior
- adapter tests:
  - non-streaming success
  - streaming success where supported
  - 429 / timeout / 5xx mapping
- HTTP tests:
  - canonical model routing
  - alias routing
  - capability checks

## Exit Criteria

`2D` is complete only when:

- ByteDance chat models are callable through `/v1/chat/completions`
- auth and endpoint handling are test-covered
- registry and provider docs are updated
- all related tests pass under `go test -race ./...`

## Non-Goals

`2D` must not implement:

- Doubao TTS
- Seedance video
- Seedream image

Those are later-phase modalities.
