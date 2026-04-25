# Phase 2B: OpenAI-Compatible Chat Provider Wave

**Status:** Implemented

## Summary

This milestone adds the lowest-friction provider wave first: DeepSeek, xAI, and Ollama. These providers are the best bridge from the Phase 1 OpenAI-style chat path into the broader Phase 2 provider matrix because they either use OpenAI-compatible wire shapes directly or require only light translation.

## Key Changes

- **DeepSeek**
  - Implement `internal/provider/deepseek/client.go` and `internal/provider/deepseek/chat.go`.
  - Use Bearer auth and the configured base URL.
  - Support JSON and streaming chat completions through the existing `modality.ChatAdapter` contract.
  - Map DeepSeek upstream errors into Polaris error types.
  - Handle `reasoning_content` consistently:
    - either strip it
    - or store it in a stable metadata extension if the chat contract already supports it by then
  - Do not invent ad hoc response fields in this milestone.

- **xAI**
  - Implement `internal/provider/xai/client.go` and `internal/provider/xai/chat.go`.
  - Treat xAI as near-OpenAI-compatible:
    - Bearer auth
    - OpenAI-style request/response mapping
    - SSE normalization when needed
  - Keep provider-specific error classification explicit rather than assuming all error bodies match OpenAI perfectly.

- **Ollama**
  - Implement `internal/provider/ollama/client.go` and `internal/provider/ollama/chat.go`.
  - No auth by default.
  - Use the configured base URL.
  - Support local-model startup behavior with high request timeout defaults.
  - Normalize Ollama streaming into the same OpenAI-compatible SSE output used by the Phase 1 chat handler.

- **Registry integration**
  - Register all three providers as real chat adapters in `internal/provider/registry.go`.
  - Preserve the provider-enable semantics:
    - DeepSeek and xAI require API keys
    - Ollama remains valid without credentials
  - Continue to use config-defined models as the sole source of registry truth.

- **Docs**
  - Update `docs/PROVIDERS.md` for the new providers:
    - auth requirements
    - base URL expectations
    - known quirks
    - timeout expectations

## Public Interfaces Added Or Changed

- no new HTTP endpoints
- `/v1/chat/completions` gains support for:
  - DeepSeek models
  - xAI models
  - Ollama models
- the registry now exposes a broader canonical model set while preserving the same alias and adapter lookup API

## Test Plan

- DeepSeek adapter tests:
  - non-streaming success
  - streaming success
  - 401 / 429 / 5xx mapping
  - timeout behavior
  - reasoning-content handling
- xAI adapter tests:
  - non-streaming success
  - streaming success
  - error mapping
- Ollama adapter tests:
  - non-streaming success
  - streaming success
  - startup-latency timeout handling
  - no-auth request path
- HTTP tests:
  - `/v1/chat/completions` against canonical model IDs for each provider
  - alias resolution to each provider
  - capability checks remain enforced

## Exit Criteria

`2B` is complete only when:

- DeepSeek, xAI, and Ollama models work end to end through `/v1/chat/completions`
- all three providers are registry-backed and config-driven
- provider docs are updated
- adapter tests pass under `go test -race ./...`
