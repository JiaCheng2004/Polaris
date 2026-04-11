# Polaris Provider Notes

This file is the operator-facing companion to `BLUEPRINT.md` §7 and §11. It records provider-specific authentication rules, compatibility quirks, and the implementation phase each provider belongs to.

## Phase 1

### OpenAI

- Auth: `Authorization: Bearer <key>`
- Primary scope: chat in Phase 1
- Later modalities: image, voice, embeddings
- Notes: standard SSE streaming and the closest OpenAI wire compatibility baseline

### Anthropic

- Auth: `x-api-key` plus `anthropic-version`
- Primary scope: chat in Phase 1
- Notes: adapter must translate Anthropic system/message structure into Polaris/OpenAI-compatible shapes

## Phase 2

### DeepSeek

- Auth: OpenAI-compatible bearer token
- Scope: chat
- Notes: reasoner output may require normalization

### Google

- Auth: API key in query string
- Scope: chat in Phase 2, image/voice/embed in Phase 3
- Notes: streaming and image generation do not use the same wire format as OpenAI

### xAI

- Auth: bearer token
- Scope: chat

### Qwen

- Auth: DashScope compatible-mode bearer token
- Scope: chat in Phase 2, image in Phase 3

### ByteDance

- Auth: Volcengine HMAC signing
- Scope: chat in Phase 2, voice/image/video later
- Notes: highest auth complexity in the provider set

### Ollama

- Auth: none
- Scope: chat
- Notes: local provider; first request may be slow while models load

## Documentation Rule

Every provider implementation PR should update this file with:

- auth setup
- modality support added in that PR
- any request or response translation quirks
- any operational gotchas operators need to know
