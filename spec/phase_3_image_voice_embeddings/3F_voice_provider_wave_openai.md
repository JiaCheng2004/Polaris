# Phase 3F: Voice Provider Wave — OpenAI

**Status:** Implemented

## Summary

This milestone activated the voice surface with the provider that supports both public Phase 3 operations: text-to-speech and speech-to-text. The shipped implementation makes the audio endpoints real without introducing streaming voice transport or full-duplex audio.

## Key Changes

- **Voice adapters**
  - Implement OpenAI TTS in `internal/provider/openai/voice.go` for:
    - `tts-1`
    - `tts-1-hd`
  - Implement OpenAI STT in the same provider package for:
    - `whisper-1`
  - Normalize provider behavior into the shared `modality.TTSRequest`, `AudioResponse`, `STTRequest`, and `TranscriptResponse` contracts.

- **Endpoint activation**
  - Implement:
    - `POST /v1/audio/speech`
    - `POST /v1/audio/transcriptions`
  - Keep the public contract aligned with `docs/API_REFERENCE.md`.
  - Return raw audio bytes for TTS with the correct `Content-Type`.
  - Return JSON, plain text, SRT, or VTT from STT depending on `response_format`.

- **Validation**
  - TTS:
    - enforce `voice` selection against model metadata
    - enforce supported `response_format`
    - enforce `speed` bounds
  - STT:
    - require multipart upload with `file`
    - enforce model-declared input formats
    - validate `response_format` and `temperature`

- **Usage integration**
  - Log TTS and STT requests through the async usage pipeline.
  - Keep token counts at zero unless the provider returns token usage.
  - Cost estimation should be request-based or provider-response-based without changing the `/v1/usage` schema.

## Public Interfaces Added Or Changed

- New live endpoints:
  - `POST /v1/audio/speech`
  - `POST /v1/audio/transcriptions`
- `GET /v1/models` must expose voice-model metadata:
  - `voices`
  - `formats`
  - voice capabilities

## Test Plan

- OpenAI TTS adapter tests:
  - mp3 and wav responses
  - voice selection validation
  - invalid speed / format
- OpenAI STT adapter tests:
  - JSON, text, SRT, and VTT responses
  - multipart upload handling
  - format validation
- Handler tests:
  - alias resolution
  - non-voice model rejection
  - capability mismatch
  - raw audio response headers
- Usage tests for TTS and STT request logging

## Exit Criteria

`3F` is complete only when:

- both voice endpoints work end to end for OpenAI models
- raw-audio and multipart flows are validated and tested
- `/v1/models` and `/v1/usage` reflect the new voice surface correctly
- docs describe the real Phase 3 voice behavior

## Non-Goals

`3F` must not add:

- ByteDance voice
- streaming voice transport
- full-duplex audio
- SDK work

Those are handled by later Phase 3 or Phase 4 steps.
