# Phase 2C: Google and Qwen Chat Provider Wave

**Status:** Implemented

## Summary

This milestone adds the medium-complexity translation providers: Google Gemini and Qwen. These providers expand the chat ecosystem significantly, but they need more translation than the OpenAI-compatible provider wave.

This step must remain focused on chat only. Image support for Google or Qwen is Phase 3 work.

## Key Changes

- **Google Gemini**
  - Implement `internal/provider/google/client.go` and `internal/provider/google/chat.go`.
  - Use query-param API key auth.
  - Translate roles:
    - Polaris `user` -> Gemini `user`
    - Polaris `assistant` -> Gemini `model`
  - Normalize Google’s chunked JSON streaming into the existing OpenAI-compatible SSE behavior.
  - Map provider errors into Polaris error classes.
  - Keep Google-specific modality expansion out of scope here. This step is chat only.

- **Qwen**
  - Implement `internal/provider/qwen/client.go` and `internal/provider/qwen/chat.go`.
  - Use DashScope-compatible auth and base URL handling.
  - Support JSON and streaming chat completions.
  - Map provider errors into Polaris error classes.
  - Keep Qwen image support explicitly out of scope for this step.

- **Registry integration**
  - Register Google and Qwen chat models in the main registry.
  - Preserve capability-driven behavior so model metadata remains the source of request gating.

- **Docs**
  - Update `docs/PROVIDERS.md` for:
    - Google auth and role translation
    - Qwen auth and DashScope mode expectations
    - chat-only Phase 2 support boundary

## Public Interfaces Added Or Changed

- no new HTTP endpoints
- `/v1/chat/completions` gains support for:
  - Google Gemini chat models
  - Qwen chat models

## Test Plan

- Google adapter tests:
  - role translation
  - non-streaming success
  - streaming normalization
  - auth and upstream-error mapping
- Qwen adapter tests:
  - non-streaming success
  - streaming success
  - error mapping
- HTTP tests:
  - canonical model routing
  - alias routing
  - capability enforcement remains correct

## Exit Criteria

`2C` is complete only when:

- Google and Qwen chat calls work end to end through the Phase 1 chat endpoint
- streaming behavior is normalized correctly for Google
- provider docs are updated
- all related tests pass under `go test -race ./...`

## Non-Goals

`2C` must not implement:

- Google image generation
- Qwen image generation
- voice, embeddings, or any other modality

Those remain Phase 3 work.
