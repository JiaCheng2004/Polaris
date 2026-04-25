# Phase 3B: Multimodal Contracts and Registry

**Status:** Implemented

## Summary

This milestone turns the non-chat modality contracts into real runtime infrastructure. It does not fully activate provider coverage yet. Its purpose is to stabilize the shared request, response, validation, and registry behavior for embeddings, images, and voice before provider-specific work begins.

## Key Changes

- **Shared modality contracts**
  - Implement real `internal/modality/embed.go`, `image.go`, and `voice.go` types and interfaces per `BLUEPRINT.md` section 6.
  - Keep the existing API reference as the wire-contract authority for field names, required fields, and response shapes.
  - Preserve the existing chat contract unchanged.

- **Registry expansion**
  - Extend the provider registry with:
    - `GetEmbedAdapter`
    - `GetImageAdapter`
    - `GetVoiceAdapter`
  - Reuse the same resolution flow already used by chat:
    - alias resolution
    - canonical lookup
    - modality check
    - allowed-model enforcement
  - Ensure model metadata remains one canonical source for modality-specific capabilities and model properties.

- **Gateway helpers**
  - Add shared request parsing and validation helpers for:
    - JSON bodies for embeddings and images
    - multipart/form-data for transcription uploads
    - raw binary audio responses for TTS
  - Add provider-independent capability checks for:
    - `generation`
    - `editing`
    - `multi_reference`
    - `tts`
    - `stt`

- **Handler scaffolding**
  - Make `handler/embed.go`, `handler/image.go`, and `handler/voice.go` real entrypoints that can resolve models, validate requests, and call adapters once provider waves land.
  - Route registration should become real, but unfinished provider/model combinations must still return explicit `model_not_found` or `capability_not_supported` errors rather than silent placeholders.

## Public Interfaces Added Or Changed

- No new provider coverage yet, but the following endpoint handlers become real routing surfaces:
  - `POST /v1/embeddings`
  - `POST /v1/images/generations`
  - `POST /v1/images/edits`
  - `POST /v1/audio/speech`
  - `POST /v1/audio/transcriptions`
- Registry interface expands beyond chat to include image, voice, and embedding adapter lookup.

## Test Plan

- Contract tests for request validation and response serialization
- Registry tests for alias resolution and modality mismatch across non-chat modalities
- Multipart parsing tests for transcription uploads
- Raw-audio response tests for TTS handlers
- Handler tests proving unimplemented provider/model pairs fail explicitly and consistently

## Exit Criteria

`3B` is complete only when:

- modality contracts for embed, image, and voice are real and stable
- registry lookup works for all Phase 3 modalities
- handlers can parse, validate, and route requests without provider-specific branching in middleware
- the new handler and registry tests pass cleanly

## Non-Goals

`3B` must not add:

- concrete embedding providers
- concrete image providers
- concrete voice providers
- SDK code

Those belong to the provider-wave and SDK steps that follow.
