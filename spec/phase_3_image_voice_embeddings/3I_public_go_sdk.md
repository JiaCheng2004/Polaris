# Phase 3I: Public Go SDK

**Status:** Implemented

## Summary

This milestone ships the first real importable Go SDK in `pkg/client`. It follows the already-implemented Polaris HTTP surface instead of inventing a parallel abstraction layer.

The SDK lands after the Phase 3 endpoint contracts are stable so it can wrap real behavior rather than placeholders.

## Key Changes

- **SDK client shape**
  - Implement `New(baseURL string, opts ...Option) (*Client, error)` in `pkg/client`.
  - Support configuration through constructor options for:
    - API key
    - timeout
    - injected `*http.Client`
  - Keep the package flat and avoid subpackages for the initial SDK release.

- **Implemented endpoint groups**
  - Chat:
    - `CreateChatCompletion`
    - `StreamChatCompletion`
  - Embeddings:
    - `CreateEmbedding`
  - Images:
    - `GenerateImage`
    - `EditImage`
  - Voice:
    - `CreateSpeech`
    - `CreateTranscription`
  - Metadata and reporting:
    - `ListModels`
    - `GetUsage`
  - Admin:
    - `CreateKey`
    - `ListKeys`
    - `DeleteKey`

- **Type boundaries**
  - Define SDK-facing request and response types in `pkg/client`.
  - Do not export `internal/modality` types through the SDK API.
  - Parse Polaris/OpenAI-style error responses into one consistent SDK error type.

- **Streaming and multipart**
  - Support SSE chat streaming with `ChatStream` and `Next`, `Chunk`, `Err`, `Close`.
  - Support multipart upload helpers for transcription requests.
  - Support raw binary TTS responses while preserving response headers needed to interpret the audio format.

- **Documentation**
  - Update `README.md` and contributor docs once the SDK is real.
  - Historical scope note: Phase 3 kept `pkg/client/video.go` as a placeholder rather than exposing unsupported APIs. Phase 4A later implemented the public video SDK surface.

## Public Interfaces Added Or Changed

- New public Go SDK in `pkg/client` for all implemented non-video Polaris endpoints
- No HTTP API changes
- Historical note: `pkg/client/video.go` remained intentionally unimplemented in Phase 3 and was added in Phase 4A

## Test Plan

- Unit tests for:
  - request encoding
  - auth header injection
  - query parameter handling
  - error decoding
- Streaming tests for chat SSE handling
- Multipart tests for transcription uploads
- Raw-binary tests for speech synthesis downloads
- End-to-end SDK smoke tests against `httptest` servers and one real local Polaris smoke path

## Exit Criteria

`3I` is complete only when:

- `pkg/client` is importable and usable for all implemented Phase 3 endpoints
- chat streaming, image, embedding, voice, usage, and key-management helpers all have tests
- the SDK uses its own stable public types rather than leaking internal packages
- docs explain how to configure and use the SDK

## Non-Goals

`3I` must not add:

- video SDK methods
- npm or pip SDKs
- non-implemented HTTP surfaces

Those remain later work.
