# Polaris API Reference

> **Version:** 2.0
> **Status:** Mixed. The Phase 1 HTTP surface is implemented: `GET /health`, `GET /ready`, `GET /v1/models`, `POST /v1/chat/completions`, and `GET /v1/usage`. The remaining sections describe the target later-phase endpoints and behavior from `BLUEPRINT.md`.
> **Authority:** Per `BLUEPRINT.md` §18, every PR that adds, modifies, or removes an endpoint MUST update this file in the same commit. If this document and the implementation disagree, the implementation is wrong OR this document is wrong — one of them must be fixed before the PR merges.

---

## Table of Contents

1. [Conventions](#1-conventions)
2. [Authentication](#2-authentication)
3. [Model Naming](#3-model-naming)
4. [Error Format](#4-error-format)
5. [Streaming Protocol](#5-streaming-protocol)
6. [Chat](#6-chat)
7. [Embeddings](#7-embeddings)
8. [Images](#8-images)
9. [Video](#9-video)
10. [Voice — TTS & STT](#10-voice--tts--stt)
11. [Models](#11-models)
12. [Usage](#12-usage)
13. [API Keys (Admin)](#13-api-keys-admin)
14. [Health & Readiness](#14-health--readiness)
15. [Metrics](#15-metrics)
16. [Full Endpoint Map](#16-full-endpoint-map)

---

## 1. Conventions

- **Base URL:** configurable via `server.host` and `server.port` in `polaris.yaml`. Default: `http://localhost:8080`.
- **Transport:** HTTP/1.1 and HTTP/2 (Gin default). TLS terminated upstream by a reverse proxy in production.
- **Content type:** `application/json` for request and response bodies unless otherwise noted (image/audio file uploads use `multipart/form-data`; audio responses return raw binary).
- **Character encoding:** UTF-8.
- **Timestamps:** Unix seconds (integer) in JSON bodies; ISO-8601 (`RFC 3339`) in log lines.
- **Request IDs:** every response carries an `X-Request-ID` header. Clients may supply their own — the server propagates it if present, generates one otherwise.
- **Failover marker:** when failover is implemented in a later phase, responses served from a fallback provider carry an `X-Polaris-Fallback: <provider/model>` header indicating which fallback answered.
- **OpenAI compatibility:** where an endpoint is implemented, Polaris follows the OpenAI wire shape where possible. In the current Phase 1 build, that applies to chat and models.

---

## 2. Authentication

Polaris supports three auth modes, selected via `auth.mode` in `polaris.yaml` (see `BLUEPRINT.md` §5.1 and §10.1):

| Mode | Header | Key source |
|---|---|---|
| `none` | — | No key required. Every request passes. Intended for local development only. |
| `static` | `Authorization: Bearer <key>` | Keys defined in `polaris.yaml` under `auth.static_keys`. SHA-256 hash lookup in-memory, O(1). |
| `multi-user` | `Authorization: Bearer <key>` | Keys stored in the database. Hashed lookup via `store.GetAPIKeyByHash`, cached in-memory with 60s TTL. |

Admin endpoints (`/v1/keys`) require `auth.mode: multi-user` and a key marked `is_admin: true`.

Per `BLUEPRINT.md` §19:
- Keys are stored as SHA-256 hashes. Plaintext keys are never persisted.
- Logs contain `key_prefix` (first 8 characters) only. Never the full key.

### Error responses from the auth layer

| Condition | HTTP | `error.type` | `error.code` |
|---|---|---|---|
| Missing `Authorization` header | 401 | `authentication_error` | `missing_api_key` |
| Header present but key not found | 401 | `authentication_error` | `invalid_api_key` |
| Key found but revoked or expired | 401 | `authentication_error` | `key_revoked` / `key_expired` |
| Key valid but not permitted to use the requested model | 403 | `permission_error` | `model_not_allowed` |
| Admin endpoint called without admin key | 403 | `permission_error` | `admin_required` |

---

## 3. Model Naming

All model references use the form `provider/model` or a configured alias (`BLUEPRINT.md` §8.5).

- **Canonical:** `openai/gpt-4o`, `anthropic/claude-sonnet-4-6`, `bytedance/seedance-2.0`, `google/nano-banana-pro`.
- **Alias:** `default-chat`, `budget-chat`, `premium-chat`, `default-image`, `default-video`, `fast-video`, `default-voice-tts`, `default-voice-stt`, `default-embed` (see `BLUEPRINT.md` §5.1 `routing.aliases`).

Resolution order per request:
1. If the string contains no `/`, treat as an alias → look up in `routing.aliases`.
2. Otherwise treat as canonical → look up in the registry.
3. Check modality matches the endpoint (chat endpoint requires a chat model, etc.).
4. Check the authenticated key's `allowed_models` glob patterns permit the model.
5. On any failure, return the appropriate error below.

| Condition | HTTP | `error.type` | `error.code` |
|---|---|---|---|
| Alias not defined | 404 | `model_not_found` | `unknown_alias` |
| `provider/model` not in registry | 404 | `model_not_found` | `unknown_model` |
| Model's modality mismatches the endpoint | 400 | `invalid_request_error` | `modality_mismatch` |
| Model lacks a capability the request needs | 400 | `capability_not_supported` | `capability_missing` |

---

## 4. Error Format

Every error response — regardless of endpoint — uses the OpenAI-compatible envelope (`BLUEPRINT.md` §8.3):

```json
{
  "error": {
    "message": "Human-readable description of what went wrong.",
    "type": "invalid_request_error",
    "code": "model_not_found",
    "param": "model"
  }
}
```

Fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| `error.message` | string | yes | Human-readable, safe to surface to end users. Must not leak secrets or internal stack traces. |
| `error.type` | string | yes | One of the canonical types below. Maps 1:1 with a Polaris error class. |
| `error.code` | string | no | Stable machine-readable subtype. Clients may switch on this. |
| `error.param` | string | no | Name of the offending request field, if applicable. |

### Canonical error types

| `error.type` | HTTP | When |
|---|---|---|
| `invalid_request_error` | 400 | Malformed JSON, missing required field, invalid enum value, param out of range. |
| `capability_not_supported` | 400 | Request uses a capability the resolved model does not declare (vision, function calling, JSON mode, etc.). |
| `authentication_error` | 401 | Missing, invalid, expired, or revoked API key. |
| `permission_error` | 403 | Key is valid but not permitted to use the requested model or admin endpoint. |
| `model_not_found` | 404 | Alias or `provider/model` is not registered. |
| `rate_limit_error` | 429 | Request would exceed the key's rate limit. Response carries `Retry-After` and `X-RateLimit-Remaining`. |
| `provider_error` | 502 | Upstream provider returned an error or malformed response. |
| `timeout_error` | 504 | Upstream provider exceeded the configured timeout. |
| `internal_error` | 500 | Unhandled server-side error. The `X-Request-ID` should accompany any bug report. |

### Failover behavior

Per `BLUEPRINT.md` §12.2, retryable errors (`rate_limit_error`, `timeout_error`, `provider_error` with 5xx status) trigger failover if `routing.fallbacks` is configured for the originating `provider/model`. Non-retryable errors (400, 401, 403, 422) never trigger failover. When a fallback answers, the response body is the fallback's normal response and the response carries `X-Polaris-Fallback: <provider/model>`.

---

## 5. Streaming Protocol

Streaming endpoints (currently chat only) use Server-Sent Events, wire-compatible with OpenAI (`BLUEPRINT.md` §8.4):

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Request-ID: <id>

data: {"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1744329600,"model":"openai/gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1744329600,"model":"openai/gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1744329600,"model":"openai/gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
```

Rules:
- The server flushes after every `data:` line.
- The final data frame before `[DONE]` MUST include `usage`.
- If an error occurs mid-stream, the server emits a single `data:` frame containing an `error` envelope (same shape as §4) and then `data: [DONE]`. The HTTP status code is still 200 because headers have already been flushed.
- Clients should parse each frame as JSON except for the literal `[DONE]` sentinel.

---

## 6. Chat

### `POST /v1/chat/completions`

Generate a chat completion. Supports streaming and non-streaming, text-only and multimodal inputs, function calling, structured outputs, and the OpenAI-compatible tool protocol.

**Auth:** required (any mode except `none`).
**Modality:** `chat`.
**Backed by:** any model registered with `modality: chat` (see `BLUEPRINT.md` §5.1 for the provider catalog).

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias. Must resolve to a chat model. |
| `messages` | array | yes | Ordered conversation. At least one element. Each element has `role` and `content`. |
| `messages[].role` | string | yes | One of `system`, `user`, `assistant`, `tool`. |
| `messages[].content` | string \| array | yes | Either a plain string or an array of content parts (see below). |
| `messages[].name` | string | no | Optional participant name. |
| `messages[].tool_call_id` | string | no | Required on `role: tool` messages. References the `id` of a prior tool call. |
| `temperature` | number | no | `0.0`–`2.0`. Provider default if omitted. |
| `top_p` | number | no | `0.0`–`1.0`. |
| `max_tokens` | integer | no | Upper bound on generated tokens. Required for Anthropic models (the adapter fills a default if omitted). |
| `stream` | boolean | no | If `true`, response is SSE per §5. |
| `tools` | array | no | OpenAI-compatible function definitions. Requires the model to declare the `function_calling` capability. |
| `tool_choice` | string \| object | no | `"auto"`, `"none"`, `"required"`, or `{"type":"function","function":{"name":"..."}}`. |
| `response_format` | object | no | `{"type":"json_object"}` or `{"type":"json_schema","json_schema":{...}}`. Requires the model to declare JSON-mode support. |
| `stop` | string \| array | no | Up to 4 stop sequences. |

**Multimodal content parts** (when `messages[].content` is an array):

| `type` | Extra fields | Capability required |
|---|---|---|
| `text` | `text` (string) | — |
| `image_url` | `image_url.url` (http(s) URL or `data:` URI), `image_url.detail` (`low`/`high`/`auto`) | `vision` |
| `input_audio` | `input_audio.data` (base64), `input_audio.format` (`wav`/`mp3`/…) | `audio_input` |

If the resolved model does not declare the required capability, the response is `400 capability_not_supported`.

#### Non-streaming response (`stream: false`)

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1744329600,
  "model": "openai/gpt-4o",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello, how can I help?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 7,
    "total_tokens": 17
  }
}
```

`finish_reason` is one of `stop`, `length`, `tool_calls`, `content_filter`. When the assistant invokes a tool, `message.content` is `null` and `message.tool_calls` is present (same shape as OpenAI).

#### Streaming response (`stream: true`)

See §5 for frame format. Chunk shape:

```json
{
  "id": "chatcmpl-abc123",
  "object": "chat.completion.chunk",
  "created": 1744329600,
  "model": "openai/gpt-4o",
  "choices": [
    {
      "index": 0,
      "delta": { "role": "assistant", "content": "Hello" },
      "finish_reason": null
    }
  ]
}
```

`usage` appears only on the final chunk. Partial `tool_calls` are emitted inside `delta.tool_calls` with the same OpenAI shape.

#### Errors

| HTTP | `type` | Cause |
|---|---|---|
| 400 | `invalid_request_error` | Bad JSON, empty `messages`, unknown `role`, `temperature` out of range, malformed `tools`. |
| 400 | `capability_not_supported` | Vision/tool/JSON-mode request against a model that lacks the capability. |
| 401 | `authentication_error` | Missing or invalid key. |
| 403 | `permission_error` | Key not permitted to use the resolved model. |
| 404 | `model_not_found` | Unknown alias or `provider/model`. |
| 429 | `rate_limit_error` | Rate limit exceeded. |
| 502 | `provider_error` | Upstream provider returned 5xx or malformed body (after retries). |
| 504 | `timeout_error` | Upstream provider exceeded configured timeout. |

#### curl

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $POLARIS_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic/claude-sonnet-4-6",
    "messages": [
      {"role": "system", "content": "You are a concise assistant."},
      {"role": "user", "content": "What is the capital of France?"}
    ],
    "max_tokens": 64
  }'
```

---

## 7. Embeddings

### `POST /v1/embeddings`

Generate embedding vectors for text inputs.

**Auth:** required.
**Modality:** `embed`.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias resolving to an embed model. |
| `input` | string \| array | yes | Either a single string or an array of strings. |
| `dimensions` | integer | no | Optional truncation (only supported by models that declare it, e.g. `text-embedding-3-large`). |
| `encoding_format` | string | no | `float` (default) or `base64`. |

#### Response body

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [0.0012, -0.0045, 0.0891, ...]
    }
  ],
  "model": "openai/text-embedding-3-small",
  "usage": {
    "prompt_tokens": 8,
    "total_tokens": 8
  }
}
```

When `encoding_format: base64`, `embedding` is a base64-encoded string of little-endian float32 values.

#### Errors

Same set as chat, minus streaming-related cases.

#### curl

```bash
curl http://localhost:8080/v1/embeddings \
  -H "Authorization: Bearer $POLARIS_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/text-embedding-3-small",
    "input": ["The quick brown fox", "jumps over the lazy dog"]
  }'
```

---

## 8. Images

### `POST /v1/images/generations`

Generate one or more images from a text prompt.

**Auth:** required.
**Modality:** `image`.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias resolving to an image model. |
| `prompt` | string | yes | Up to the model's prompt length limit. |
| `n` | integer | no | Number of images, `1`–`10`. Default `1`. Not all providers honor `n>1`. |
| `size` | string | no | e.g. `1024x1024`, `1792x1024`. Model-specific valid values. |
| `quality` | string | no | `standard` or `hd`. Model-specific. |
| `style` | string | no | `vivid` or `natural`. Model-specific. |
| `response_format` | string | no | `url` (default) or `b64_json`. |
| `reference_images` | array | no | Array of image URLs or base64 data. Requires the model to declare `multi_reference` (e.g. `google/nano-banana-pro`, `bytedance/seedream-4.5`). |

#### Response body

```json
{
  "created": 1744329600,
  "data": [
    {
      "url": "https://...",
      "revised_prompt": "A concise re-statement of the prompt the model actually rendered."
    }
  ]
}
```

When `response_format: b64_json`, each item carries `b64_json` instead of `url`.

### `POST /v1/images/edits`

Edit an existing image under a text instruction. Same response shape as generation.

**Content type:** `multipart/form-data`.

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | Must declare the `editing` capability. |
| `image` | file | yes | PNG or JPEG. Size/format constraints are model-specific. |
| `prompt` | string | yes | Edit instruction. |
| `mask` | file | no | PNG with alpha channel marking the region to edit. |
| `n` | integer | no | As above. |
| `size` | string | no | As above. |
| `response_format` | string | no | As above. |

#### Errors

| HTTP | `type` | Cause |
|---|---|---|
| 400 | `invalid_request_error` | Missing `prompt`, unsupported `size`, malformed upload. |
| 400 | `capability_not_supported` | Model lacks `generation`, `editing`, or `multi_reference`. |
| 401/403/404/429/502/504 | — | Standard set. |

#### curl

```bash
# Generation
curl http://localhost:8080/v1/images/generations \
  -H "Authorization: Bearer $POLARIS_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "google/nano-banana-pro",
    "prompt": "A watercolor illustration of a lighthouse at dusk",
    "size": "1024x1024",
    "n": 1
  }'

# Editing
curl http://localhost:8080/v1/images/edits \
  -H "Authorization: Bearer $POLARIS_KEY" \
  -F "model=openai/gpt-image-1" \
  -F "image=@./original.png" \
  -F "mask=@./mask.png" \
  -F "prompt=Replace the sky with a starfield"
```

---

## 9. Video

Video generation is asynchronous — submit a job, poll for completion (`BLUEPRINT.md` §6.3). The endpoint surface is Polaris-defined; no OpenAI equivalent exists.

### `POST /v1/video/generations`

Submit a video generation job.

**Auth:** required.
**Modality:** `video`.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias resolving to a video model. |
| `prompt` | string | yes | Scene description. |
| `duration` | integer | no | Seconds, `4`–`15`. Provider default if omitted. Must not exceed model's `max_duration`. |
| `aspect_ratio` | string | no | e.g. `16:9`, `9:16`, `1:1`. |
| `resolution` | string | no | e.g. `720p`, `1080p`. Must be listed in the model's `resolutions`. |
| `first_frame` | string | no | URL or base64 image anchoring the opening frame. Requires `image_to_video` capability. |
| `last_frame` | string | no | URL or base64 image anchoring the closing frame. |
| `reference_images` | array | no | Additional reference images (style/subject anchors). |
| `reference_videos` | array | no | Reference videos for style/motion transfer. |
| `audio` | string | no | URL or base64 audio to sync against. |
| `with_audio` | boolean | no | Generate native audio track (requires `native_audio` capability). |

#### Response body (job submitted)

```json
{
  "job_id": "vid_01HXYZ...",
  "status": "queued",
  "estimated_time": 45,
  "model": "bytedance/seedance-2.0"
}
```

`status` is one of `queued`, `processing`, `completed`, `failed`.
`estimated_time` is seconds until expected completion (best-effort).

### `GET /v1/video/generations/:id`

Poll a previously submitted job.

#### Response body

```json
{
  "job_id": "vid_01HXYZ...",
  "status": "completed",
  "progress": 1.0,
  "result": {
    "video_url": "https://...",
    "audio_url": "https://...",
    "duration": 8,
    "width": 1920,
    "height": 1080
  },
  "error": null
}
```

- `progress` is `0.0`–`1.0`.
- `result` is populated only when `status == "completed"`.
- `error` is populated only when `status == "failed"` and carries a nested `{type, message, code}` envelope matching §4.

### `DELETE /v1/video/generations/:id`

Cancel a running job. Returns `204 No Content` on success. Jobs already in `completed` or `failed` state return `409 invalid_request_error / job_immutable`.

#### Errors

| HTTP | `type` | Cause |
|---|---|---|
| 400 | `invalid_request_error` | Missing `prompt`, `duration` out of range, `resolution` not supported, conflicting frame anchors. |
| 400 | `capability_not_supported` | Request uses `first_frame`/`reference_*`/`with_audio` but the model lacks the capability. |
| 404 | `model_not_found` | Unknown model or alias. |
| 404 | `invalid_request_error / job_not_found` | Polling or cancelling an unknown job id. |
| 409 | `invalid_request_error / job_immutable` | Cancelling a terminal job. |
| 429/502/504 | — | Standard set. |

#### curl

```bash
# Submit
curl http://localhost:8080/v1/video/generations \
  -H "Authorization: Bearer $POLARIS_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "bytedance/seedance-2.0",
    "prompt": "A drone flyover of a snowy mountain range at sunrise",
    "duration": 8,
    "resolution": "1080p",
    "with_audio": true
  }'

# Poll
curl http://localhost:8080/v1/video/generations/vid_01HXYZ... \
  -H "Authorization: Bearer $POLARIS_KEY"

# Cancel
curl -X DELETE http://localhost:8080/v1/video/generations/vid_01HXYZ... \
  -H "Authorization: Bearer $POLARIS_KEY"
```

---

## 10. Voice — TTS & STT

### `POST /v1/audio/speech`

Text-to-speech. Returns a raw audio body.

**Auth:** required.
**Modality:** `voice` (TTS).
**Response content type:** `audio/mpeg`, `audio/wav`, `audio/ogg`, `audio/flac`, or `audio/pcm`, matching `response_format`.

#### Request body (JSON)

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | Must declare the `tts` capability. |
| `input` | string | yes | Text to synthesize. |
| `voice` | string | yes | Voice ID. Must be listed in the model's `voices`. |
| `response_format` | string | no | `mp3` (default), `opus`, `aac`, `flac`, `wav`, `pcm`. |
| `speed` | number | no | `0.25`–`4.0`. Default `1.0`. |

#### Response

Raw audio bytes. The `Content-Type` header reflects `response_format`.

### `POST /v1/audio/transcriptions`

Speech-to-text.

**Auth:** required.
**Modality:** `voice` (STT).
**Content type:** `multipart/form-data`.

#### Request form fields

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | Must declare the `stt` capability. |
| `file` | file | yes | Audio file. Accepted formats are the intersection of the client-provided extension and the model's declared `formats`. |
| `language` | string | no | ISO-639-1 code (e.g. `en`, `zh`). Improves accuracy when provided. |
| `response_format` | string | no | `json` (default), `text`, `srt`, `vtt`. |
| `temperature` | number | no | `0.0`–`1.0`. |

#### Response (`response_format: json`)

```json
{
  "text": "Hello, this is a transcription.",
  "language": "en",
  "duration": 3.42,
  "segments": [
    {
      "id": 0,
      "start": 0.0,
      "end": 3.42,
      "text": "Hello, this is a transcription."
    }
  ]
}
```

For `text`, the response body is plain UTF-8. For `srt` and `vtt`, the body is the subtitle file.

#### Errors

| HTTP | `type` | Cause |
|---|---|---|
| 400 | `invalid_request_error` | Missing `input`/`file`, unsupported `voice`, unsupported audio format. |
| 400 | `capability_not_supported` | Model lacks `tts` or `stt`. |
| 401/403/404/429/502/504 | — | Standard set. |

#### curl

```bash
# TTS
curl http://localhost:8080/v1/audio/speech \
  -H "Authorization: Bearer $POLARIS_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/tts-1",
    "input": "Polaris is online.",
    "voice": "nova",
    "response_format": "mp3"
  }' \
  --output greeting.mp3

# STT
curl http://localhost:8080/v1/audio/transcriptions \
  -H "Authorization: Bearer $POLARIS_KEY" \
  -F "model=openai/whisper-1" \
  -F "file=@./recording.wav" \
  -F "response_format=json"
```

---

## 11. Models

### `GET /v1/models`

List every model registered in the running Polaris instance, with its modality and capabilities.

**Auth:** required.

#### Response body

```json
{
  "object": "list",
  "data": [
    {
      "id": "openai/gpt-4o",
      "object": "model",
      "provider": "openai",
      "modality": "chat",
      "capabilities": ["vision", "function_calling", "streaming", "audio_input", "audio_output"],
      "context_window": 128000,
      "max_output_tokens": 16384
    },
    {
      "id": "bytedance/seedance-2.0",
      "object": "model",
      "provider": "bytedance",
      "modality": "video",
      "capabilities": ["text_to_video", "image_to_video", "reference_images", "native_audio"],
      "max_duration": 15,
      "resolutions": ["720p", "1080p"]
    }
  ]
}
```

The field set for each entry depends on the modality and mirrors the model's config block in `polaris.yaml`. Aliases are NOT returned by this endpoint — use `GET /v1/models?include_aliases=true` to include alias entries, where `id` is the alias name and an additional `resolves_to` field points at the canonical `provider/model`.

#### Errors

Standard auth errors only.

#### curl

```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer $POLARIS_KEY"
```

---

## 12. Usage

### `GET /v1/usage`

Return usage statistics for the authenticated key over a time range.

**Auth:** required.

#### Query parameters

| Name | Type | Required | Notes |
|---|---|---|---|
| `from` | string (RFC 3339) | no | Start of the window (inclusive). Default: 30 days ago. |
| `to` | string (RFC 3339) | no | End of the window (exclusive). Default: now. |
| `model` | string | no | Filter to a specific `provider/model`. |
| `modality` | string | no | Filter to `chat`, `image`, `video`, `voice`, or `embed`. |
| `group_by` | string | no | `day` (default) or `model`. |

#### Response body

```json
{
  "from": "2026-03-12T00:00:00Z",
  "to": "2026-04-11T00:00:00Z",
  "total_requests": 1423,
  "total_tokens": 892145,
  "total_cost_usd": 18.42,
  "by_day": [
    { "date": "2026-04-10", "requests": 62, "tokens": 41203, "cost_usd": 0.84 }
  ],
  "by_model": null
}
```

When `group_by=model`, `by_day` is `null` and `by_model` contains an array of `{ "model": "...", "requests": N, "tokens": N, "cost_usd": N }` entries.

`cost_usd` is an *estimate* computed from the cost tables in `internal/gateway/middleware/usage.go`. It is NOT an authoritative bill.

#### Errors

Standard auth errors plus `400 invalid_request_error` if `from > to` or the date format is invalid.

#### curl

```bash
curl "http://localhost:8080/v1/usage?from=2026-04-01T00:00:00Z&group_by=model" \
  -H "Authorization: Bearer $POLARIS_KEY"
```

---

## 13. API Keys (Admin)

Only available when `auth.mode: multi-user`. All endpoints below require an **admin** key (`is_admin: true`).

### `POST /v1/keys`

Create a new API key.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | Human-readable label. |
| `owner_id` | string | no | Free-form identifier for the downstream user/tenant. |
| `rate_limit` | string | no | e.g. `100/min`, `10000/day`. Defaults to `cache.rate_limit.default`. |
| `allowed_models` | array | no | Glob patterns. Default `["*"]` (all models). |
| `is_admin` | boolean | no | Default `false`. |
| `expires_at` | string (RFC 3339) | no | Optional expiration. |

#### Response body

```json
{
  "id": "key_01HXYZ...",
  "name": "production-discord-bot",
  "key": "polaris-sk-live-abcdef1234567890...",
  "key_prefix": "polaris-",
  "owner_id": "discord-bot",
  "rate_limit": "1000/min",
  "allowed_models": ["openai/*", "anthropic/*"],
  "is_admin": false,
  "created_at": "2026-04-11T12:34:56Z",
  "expires_at": null
}
```

**The full `key` is returned EXACTLY ONCE at creation time.** It is NOT recoverable afterwards. Subsequent `GET /v1/keys` responses omit `key` and return `key_prefix` only.

### `GET /v1/keys`

List API keys (without the plaintext key).

#### Query parameters

| Name | Type | Notes |
|---|---|---|
| `owner_id` | string | Filter to one owner. |
| `include_revoked` | boolean | Default `false`. |

#### Response body

```json
{
  "object": "list",
  "data": [
    {
      "id": "key_01HXYZ...",
      "name": "production-discord-bot",
      "key_prefix": "polaris-",
      "owner_id": "discord-bot",
      "rate_limit": "1000/min",
      "allowed_models": ["openai/*", "anthropic/*"],
      "is_admin": false,
      "created_at": "2026-04-11T12:34:56Z",
      "last_used_at": "2026-04-11T14:00:12Z",
      "expires_at": null,
      "is_revoked": false
    }
  ]
}
```

### `DELETE /v1/keys/:id`

Revoke a key. Returns `204 No Content` on success. The key row is retained for audit purposes with `is_revoked: true`; subsequent auth attempts with that key return `401 authentication_error / key_revoked`.

#### Errors

| HTTP | `type` | Cause |
|---|---|---|
| 400 | `invalid_request_error` | Missing `name`, bad `rate_limit` format, invalid glob pattern. |
| 401 | `authentication_error` | — |
| 403 | `permission_error / admin_required` | Caller is not an admin key, or `auth.mode != multi-user`. |
| 404 | `invalid_request_error / key_not_found` | Revoking a non-existent id. |

#### curl

```bash
# Create
curl http://localhost:8080/v1/keys \
  -H "Authorization: Bearer $POLARIS_ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production-discord-bot",
    "owner_id": "discord-bot",
    "rate_limit": "1000/min",
    "allowed_models": ["openai/*", "anthropic/*"]
  }'

# List
curl http://localhost:8080/v1/keys \
  -H "Authorization: Bearer $POLARIS_ADMIN_KEY"

# Revoke
curl -X DELETE http://localhost:8080/v1/keys/key_01HXYZ... \
  -H "Authorization: Bearer $POLARIS_ADMIN_KEY"
```

---

## 14. Health & Readiness

### `GET /health`

Liveness probe. Always returns `200 OK` with `{"status":"ok"}` as long as the HTTP server is accepting connections. **No auth.**

### `GET /ready`

Readiness probe. Returns `200 OK` only when:
- the store is reachable (`store.Ping` succeeds), AND
- the cache is reachable (memory always succeeds; Redis requires a PING), AND
- at least one provider adapter is registered.

Response body:

```json
{
  "status": "ready",
  "store": "ok",
  "cache": "ok",
  "providers": 2
}
```

`providers` is the number of registered providers in the current runtime build. On failure returns `503 Service Unavailable` with the same body shape, where failing components carry an error string instead of `"ok"`. **No auth.**

---

## 15. Metrics

### `GET /metrics`

Planned for a later phase. Prometheus text-format metrics will be exposed when `observability.metrics.enabled: true` and the metrics subsystem is implemented.

Metric catalog (`BLUEPRINT.md` §13.2):

| Metric | Type | Labels |
|---|---|---|
| `polaris_requests_total` | counter | `model`, `modality`, `status`, `provider` |
| `polaris_request_duration_seconds` | histogram | `model`, `modality`, `provider` |
| `polaris_provider_latency_seconds` | histogram | `model`, `provider` |
| `polaris_tokens_total` | counter | `model`, `provider`, `direction` |
| `polaris_estimated_cost_usd` | counter | `model`, `provider` |
| `polaris_rate_limit_hits_total` | counter | `key_id` |
| `polaris_provider_errors_total` | counter | `provider`, `error_type` |
| `polaris_failovers_total` | counter | `from_model`, `to_model` |
| `polaris_active_streams` | gauge | `model`, `provider` |

---

## 16. Full Endpoint Map

| Method | Path | Modality | Auth | Status | Section |
|---|---|---|---|---|---|
| `POST` | `/v1/chat/completions` | chat | required | implemented | [§6](#6-chat) |
| `POST` | `/v1/embeddings` | embed | required | planned | [§7](#7-embeddings) |
| `POST` | `/v1/images/generations` | image | required | planned | [§8](#8-images) |
| `POST` | `/v1/images/edits` | image | required | planned | [§8](#8-images) |
| `POST` | `/v1/video/generations` | video | required | planned | [§9](#9-video) |
| `GET` | `/v1/video/generations/:id` | video | required | planned | [§9](#9-video) |
| `DELETE` | `/v1/video/generations/:id` | video | required | planned | [§9](#9-video) |
| `POST` | `/v1/audio/speech` | voice (TTS) | required | planned | [§10](#10-voice--tts--stt) |
| `POST` | `/v1/audio/transcriptions` | voice (STT) | required | planned | [§10](#10-voice--tts--stt) |
| `GET` | `/v1/models` | — | required | implemented | [§11](#11-models) |
| `GET` | `/v1/usage` | — | required | implemented | [§12](#12-usage) |
| `POST` | `/v1/keys` | — | admin | planned | [§13](#13-api-keys-admin) |
| `GET` | `/v1/keys` | — | admin | planned | [§13](#13-api-keys-admin) |
| `DELETE` | `/v1/keys/:id` | — | admin | planned | [§13](#13-api-keys-admin) |
| `GET` | `/health` | — | none | implemented | [§14](#14-health--readiness) |
| `GET` | `/ready` | — | none | implemented | [§14](#14-health--readiness) |
| `GET` | `/metrics` | — | none | planned | [§15](#15-metrics) |

---

*This document is versioned alongside the code. Any change to an endpoint's contract — path, body schema, response shape, error semantics, auth requirements — MUST land in the same commit as the code change.*
