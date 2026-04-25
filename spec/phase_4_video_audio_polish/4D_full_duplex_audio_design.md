# 4D Full-Duplex Audio Design

This document is the original Phase 4D design record. The shared contract defined here was later activated in the runtime; use `docs/API_REFERENCE.md`, `docs/CONFIGURATION.md`, and `config/polaris.example.yaml` for the current shipped behavior.

## Summary

This milestone defined the full-duplex audio contract for Polaris before the runtime implementation landed.

Locked decisions:

- Polaris-native protocol
- create-then-connect session flow
- WebSocket transport
- JSON event envelopes
- base64 audio payloads
- stateless signed session ids and client secrets

At the time of this design milestone, the section was **design-only**. The runtime has since shipped the same session contract with provider-backed cascaded execution.

## Public Surface

Reserved bootstrap endpoint:

- `POST /v1/audio/sessions`

Reserved WebSocket transport:

- `GET /v1/audio/sessions/:id/ws`

Planned session-create response:

```json
{
  "id": "audsess_01HXYZ...",
  "object": "audio.session",
  "model": "openai/realtime-voice",
  "expires_at": 1712699999,
  "websocket_url": "wss://gateway.example/v1/audio/sessions/audsess_01HXYZ.../ws",
  "client_secret": "audsec_01HXYZ..."
}
```

## Shared Contract

`internal/modality/audio.go` is the source of truth for the design contract.

Core abstractions:

- `AudioAdapter.Connect(ctx, *AudioSessionConfig) (AudioSession, error)`
- `AudioSession.Send(AudioClientEvent) error`
- `AudioSession.Events() <-chan AudioServerEvent`
- `AudioSession.Close() error`

Core types:

- `AudioSessionConfig`
- `AudioSessionDescriptor`
- `TurnDetectionConfig`
- `AudioResponseConfig`
- `AudioClientEvent`
- `AudioServerEvent`
- `AudioUsage`
- `AudioError`

Defaults:

- `input_audio_format: pcm16`
- `output_audio_format: pcm16`
- `sample_rate_hz: 16000`

Turn detection fields:

- `mode`
- `silence_ms`
- `prefix_padding_ms`

## Event Vocabulary

Client events:

- `session.update`
- `input_audio.append`
- `input_audio.commit`
- `input_text`
- `response.create`
- `response.cancel`
- `session.close`

Server events:

- `session.created`
- `session.updated`
- `input_audio.committed`
- `response.audio.delta`
- `response.audio.done`
- `response.transcript.delta`
- `response.transcript.done`
- `response.text.delta`
- `response.text.done`
- `response.completed`
- `error`

Envelope rules:

- every event carries `type`
- `event_id` is optional
- audio payloads use base64 `audio`
- text payloads use `text` or `transcript`
- terminal provider/accounting metadata uses `usage` and `error`

## Historical Runtime Boundaries

During the 4D design milestone, the runtime intentionally did **not**:

- register `/v1/audio/sessions`
- register `/v1/audio/sessions/:id/ws`
- expose `modality: audio` models in `/v1/models`
- instantiate audio adapters in the provider registry
- expose audio-session methods in `pkg/client`

Guardrails:

- `modality: audio` remains valid in shared types
- the runnable registry treated it as unsupported in 4D
- aliases pointing to audio models warned and remained unavailable in 4D
