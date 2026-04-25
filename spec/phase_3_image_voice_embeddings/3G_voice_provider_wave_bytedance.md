# Phase 3G: Voice Provider Wave — ByteDance

**Status:** Implemented

## Summary

This milestone completed the Phase 3 ByteDance voice wave. The shipped implementation extends the `3F` voice surface with ByteDance one-shot TTS and synchronous STT without changing the public provider-neutral voice contract.

## Key Changes

- **ByteDance TTS**
  - Implement ByteDance TTS in `internal/provider/bytedance/voice.go` for `doubao-tts`.
  - Support one-shot HTTP synthesis only.
  - Normalize responses into the existing raw-audio response contract from `3F`.

- **ByteDance STT**
  - Implement ByteDance synchronous STT in the same provider package for `doubao-asr-flash`.
  - Normalize provider JSON into Polaris `json`, `text`, `srt`, and `vtt` responses.
  - Keep the public multipart transcription contract unchanged.

- **Contract decision**
  - Defer public voice cloning.
  - Keep the current `/v1/audio/speech` contract unchanged.
  - Do not add provider-specific request fields for clone references or uploaded reference audio in Phase 3.
  - If model metadata still advertises `voice_cloning`, document it as not yet addressable through the public Phase 3 request shape.

- **Validation and routing**
  - Register ByteDance voice models in the shared voice-adapter registry path.
  - Reuse the same response-format validation and audio-response handling established in `3F`.
  - Preserve the same auth, alias, and allowed-model behavior as other modalities.

- **Docs**
  - Update provider docs to describe ByteDance TTS/STT setup and the deferred status of public voice cloning.
  - Reconcile API docs and model metadata notes so the public contract remains truthful.

## Public Interfaces Added Or Changed

- No new endpoint paths beyond the voice routes activated in `3F`
- Expanded runtime behavior for:
  - `POST /v1/audio/speech`
  - `POST /v1/audio/transcriptions`
- `GET /v1/models` must expose the ByteDance voice models and their voice capabilities.

## Test Plan

- ByteDance TTS adapter tests:
  - successful synthesis
  - provider-specific error mapping
  - format negotiation
- ByteDance STT adapter tests:
  - successful transcription
  - format validation
  - provider-specific error mapping
- Handler tests:
  - alias resolution
  - allowed-model enforcement
  - TTS and STT routing through the shared voice handlers
- Docs tests or assertions where needed to ensure the public contract does not claim clone-input support

## Exit Criteria

`3G` is complete only when:

- ByteDance TTS and STT work through the existing voice endpoints
- the public voice contract remains provider-neutral
- no public request field is added for voice cloning
- docs and tests make the deferred status of clone-input support explicit

## Non-Goals

`3G` must not add:

- public voice cloning input
- audio streaming or full-duplex audio
- SDK work

Those remain later work.
