# Phase 3C: Embeddings Surface — OpenAI and Google

**Status:** Implemented

## Summary

This milestone delivers the first non-chat modality through the OpenAI-compatible embeddings endpoint. It uses the two providers already declared in the architecture and config: OpenAI and Google.

Embeddings land first because they are synchronous, JSON-only, and lower-friction than image or voice while still exercising the new Phase 3 contract and registry path.

## Key Changes

- **Embed adapters**
  - Implement OpenAI embeddings in `internal/provider/openai/embed.go`.
  - Implement Google embeddings in `internal/provider/google/embed.go`.
  - Normalize both providers into the shared `modality.EmbedRequest` and `modality.EmbedResponse` contracts.

- **Endpoint activation**
  - Implement `POST /v1/embeddings` exactly as documented in `docs/API_REFERENCE.md`.
  - Support:
    - single string input and array input, normalized into the internal slice form
    - optional `dimensions`
    - optional `encoding_format`
  - Enforce that the requested model is an embedding model and that the authenticated key is allowed to use it.

- **Usage integration**
  - Record embedding requests through the existing async usage pipeline.
  - Populate token counts when the provider returns them.
  - Keep the existing `/v1/usage` shape unchanged.

- **Docs and config**
  - Mark `/v1/embeddings` as implemented in `docs/API_REFERENCE.md` only when handler and provider tests are green.
  - Ensure `docs/PROVIDERS.md` documents OpenAI and Google embedding quirks and config expectations.

## Public Interfaces Added Or Changed

- New live endpoint:
  - `POST /v1/embeddings`
- `GET /v1/models` must include embedding models with `dimensions` metadata.
- `GET /v1/usage` must count embedding requests and embed token usage.

## Test Plan

- OpenAI embed adapter tests with `httptest`
- Google embed adapter tests with `httptest`
- Handler tests for:
  - single input
  - batched input
  - dimensions override
  - alias resolution
  - non-embed model rejection
  - permission denial
- Usage tests confirming embedding requests appear in aggregated usage

## Exit Criteria

`3C` is complete only when:

- OpenAI and Google embedding models are registry-backed
- `POST /v1/embeddings` works end to end
- embedding usage is logged through the normal async path
- docs and tests match the real behavior

## Non-Goals

`3C` must not add:

- images
- voice
- SDK code
- multi-user auth changes beyond what `3A` already introduced

Those are covered by later Phase 3 steps.
