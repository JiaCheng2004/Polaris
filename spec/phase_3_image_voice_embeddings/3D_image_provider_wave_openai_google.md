# Phase 3D: Image Provider Wave — OpenAI and Google

**Status:** Implemented

## Summary

This milestone activates the image surface with the first provider wave: OpenAI and Google. It makes both image endpoints real and establishes the shared behavior for generation, editing, and multi-reference capability enforcement.

## Key Changes

- **Image adapters**
  - Implement OpenAI image support in `internal/provider/openai/image.go` for:
    - `gpt-image-1`
    - `dall-e-3`
  - Implement Google image support in `internal/provider/google/image.go` for:
    - `nano-banana-2`
    - `nano-banana-pro`
  - Normalize all responses into the shared image response shape with `url` or `b64_json` entries.

- **Endpoint activation**
  - Implement:
    - `POST /v1/images/generations`
    - `POST /v1/images/edits`
  - Keep request and response bodies aligned with `docs/API_REFERENCE.md`.
  - Enforce capability checks before provider calls:
    - generation requests require `generation`
    - edit requests require `editing`
    - reference-image inputs require `multi_reference` if more than one reference is supplied

- **Provider-specific rules**
  - OpenAI:
    - `dall-e-3` supports generation only
    - `gpt-image-1` supports generation and editing
  - Google:
    - `nano-banana-2` supports generation and editing
    - `nano-banana-pro` supports generation, editing, and multi-reference

- **Usage and metadata**
  - Log image requests through the existing async usage path.
  - Keep token fields at zero when the provider does not return token usage.
  - Surface image model metadata from `GET /v1/models`:
    - `output_formats`
    - image capabilities

## Public Interfaces Added Or Changed

- New live endpoints:
  - `POST /v1/images/generations`
  - `POST /v1/images/edits`
- `GET /v1/models` must return image-specific metadata.
- `GET /v1/usage` must include image requests in request counts and estimated cost.

## Test Plan

- OpenAI image adapter tests:
  - generation
  - edit
  - generation-only model rejecting edits
- Google image adapter tests:
  - generation
  - edit
  - multi-reference behavior for `nano-banana-pro`
- Handler tests:
  - alias resolution
  - invalid image model
  - capability mismatch
  - response format normalization
- Usage tests for image request logging and cost aggregation

## Exit Criteria

`3D` is complete only when:

- both image endpoints work end to end for OpenAI and Google models
- capability gating is enforced before provider calls
- `/v1/models` and `/v1/usage` reflect the new image surface correctly
- docs and tests match the implementation

## Non-Goals

`3D` must not add:

- ByteDance image support
- Qwen image support
- voice
- SDK work

Those are covered by later Phase 3 steps.
