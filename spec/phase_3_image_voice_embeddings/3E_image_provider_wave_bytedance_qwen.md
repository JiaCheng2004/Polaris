# Phase 3E: Image Provider Wave — ByteDance and Qwen

**Status:** Implemented

## Summary

This milestone completes the Phase 3 image provider matrix. It extends the image endpoints already activated in `3D` to ByteDance and Qwen without changing the public wire contract.

## Key Changes

- **Image adapters**
  - Implement ByteDance image support in `internal/provider/bytedance/image.go` for `seedream-4.5`.
  - Implement Qwen image support in `internal/provider/qwen/image.go` for `qwen-image-2.0`.
  - Normalize both providers into the same image response shape already established in `3D`.

- **Provider-specific behavior**
  - ByteDance `seedream-4.5`:
    - generation
    - editing
    - multi-reference
  - Qwen `qwen-image-2.0`:
    - generation
    - editing
    - no multi-reference support unless the config and model metadata are extended later

- **Registry and validation**
  - Register both providers in the image-adapter registry path.
  - Reuse the same capability checks and request validation from `3D`.
  - Preserve the same error behavior for unsupported edit or multi-reference requests.

- **Docs**
  - Update provider docs so Phase 3 image coverage is complete across OpenAI, Google, ByteDance, and Qwen.
  - Mark the image surface as fully implemented for the Phase 3 provider matrix only after handler and provider tests pass.

## Public Interfaces Added Or Changed

- No new endpoints beyond the image routes activated in `3D`
- Expanded runtime behavior for:
  - `POST /v1/images/generations`
  - `POST /v1/images/edits`
- `GET /v1/models` must expose ByteDance and Qwen image models with their declared capabilities and output formats.

## Test Plan

- ByteDance image adapter tests:
  - generation
  - edit
  - multi-reference
- Qwen image adapter tests:
  - generation
  - edit
  - multi-reference rejection
- Handler tests:
  - alias resolution to ByteDance/Qwen models
  - capability mismatch
  - provider-specific error mapping
- Usage tests confirming ByteDance and Qwen image requests are logged correctly

## Exit Criteria

`3E` is complete only when:

- all Phase 3 image providers are registry-backed
- the shared image endpoints work for all declared Phase 3 image models
- error mapping and capability validation remain consistent across providers
- docs and tests describe the full image matrix truthfully

## Non-Goals

`3E` must not add:

- voice
- embeddings changes beyond existing usage integration
- SDK work

Those are handled by later Phase 3 steps.
