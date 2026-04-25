# Polaris API Reference

> **Version:** 2.1.0
> **Status:** Phase 4 video, full-duplex audio sessions, broad sync response caching, Phase 5A music, and provider-family hardening are live in code. The current repo state is a `v2.1.0` close-out candidate; worktree consolidation and source-of-truth alignment are complete, and final release readiness is gated by repo-local validation plus live-provider proof where credentials, quota, and plan access are available. Production Postgres/Redis load validation is optional operator proof for service deployments. ElevenLabs music stays implemented behind the same API but is treated as preview until explicitly opted into live smoke.
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
10. [Music](#10-music)
11. [Voice — TTS & STT](#11-voice--tts--stt)
12. [Models](#12-models)
13. [Usage](#13-usage)
14. [Control Plane & API Keys](#14-control-plane--api-keys)
15. [MCP Broker](#15-mcp-broker)
16. [Health & Readiness](#16-health--readiness)
17. [Metrics](#17-metrics)
18. [Full Endpoint Map](#18-full-endpoint-map)

---

## 1. Conventions

- **Base URL:** configurable via `server.host` and `server.port` in `polaris.yaml`. Default: `http://localhost:8080`.
- **Transport:** HTTP/1.1 and HTTP/2 (Gin default). TLS terminated upstream by a reverse proxy in production.
- **Content type:** `application/json` for request and response bodies unless otherwise noted (image/audio file uploads use `multipart/form-data`; audio responses return raw binary).
- **Character encoding:** UTF-8.
- **Timestamps:** Unix seconds (integer) in JSON bodies; ISO-8601 (`RFC 3339`) in log lines.
- **Request IDs:** every response carries an `X-Request-ID` header. Clients may supply their own — the server propagates it if present, generates one otherwise.
- **Trace IDs:** when tracing is enabled, every response also carries `X-Trace-ID`. This is the span trace identifier for correlating logs, traces, and audit events.
- **Failover marker:** responses served from a fallback provider carry an `X-Polaris-Fallback: <provider/model>` header indicating which fallback answered.
- **Cache marker:** cache-aware synchronous endpoints may carry `X-Polaris-Cache: hit`, `miss`, or `bypass`.
- **OpenAI compatibility:** where an endpoint is implemented, Polaris follows the OpenAI wire shape where possible. In the current build, that applies to chat, models, and the OpenAI-style administrative error envelope.
- **Machine-readable contract:** [`spec/openapi/polaris.v1.yaml`](../spec/openapi/polaris.v1.yaml) is the OpenAPI companion for this human reference. Endpoint changes must keep the implementation, this file, the OpenAPI spec, and contract fixtures in sync.

---

## 2. Authentication

Polaris supports five auth modes, selected via `auth.mode` in `polaris.yaml` (see `BLUEPRINT.md` §5.1 and §10.1):

| Mode | Header | Key source |
|---|---|---|
| `none` | — | No key required. Every request passes. Intended for local development only. |
| `static` | `Authorization: Bearer <key>` | Keys defined in `polaris.yaml` under `auth.static_keys`. SHA-256 hash lookup in-memory, O(1). |
| `external` | `X-Polaris-External-Auth`, `X-Polaris-External-Auth-Timestamp`, `X-Polaris-External-Auth-Signature` | Bring-your-own-auth mode. Your platform owns login, OAuth, SMS OTP, sessions, and user lifecycle. Polaris verifies signed claims and turns them into request policy context. |
| `virtual_keys` | `Authorization: Bearer <key>` | Preferred production mode. Polaris virtual keys stored in the database. Hashed lookup via `store.GetVirtualKeyByHash`, cached in-memory with 60s TTL, then expanded into project / policy / budget context. |
| `multi-user` | `Authorization: Bearer <key>` | Compatibility mode for older database-backed API key rows. Hashed lookup via `store.GetAPIKeyByHash`, cached in-memory with 60s TTL. |

### External Auth

`auth.mode: external` is the integration path for platforms that already have their own authentication. Polaris does not implement Google OAuth, SMS OTP, enterprise SSO, or application sessions directly. Instead, the host platform validates the user, builds a small claims document, signs it with a shared secret, and forwards the request to Polaris.

The built-in external provider is `signed_headers`:

| Header | Value |
|---|---|
| `X-Polaris-External-Auth` | Base64url-encoded JSON claims. |
| `X-Polaris-External-Auth-Timestamp` | Unix seconds when the claims were signed. |
| `X-Polaris-External-Auth-Signature` | `v1=<hex hmac-sha256>` over `timestamp + "\n" + encoded_claims`. Bare hex is also accepted. |

Supported claim fields:

| Claim | Type | Behavior |
|---|---|---|
| `sub` | string | Required. External subject/user ID. Used as `OwnerID`; `key_id` defaults to `external:<sub>`. |
| `project_id` | string | Optional Polaris project ID for usage, budgets, audit context, and control-plane ownership. |
| `key_id` | string | Optional stable key identity for logs/audit. |
| `key_prefix` | string | Optional non-secret display prefix for logs/audit. |
| `is_admin` | boolean | Allows control-plane endpoints when true. |
| `rate_limit` | string | Optional per-identity limit such as `60/min`. |
| `allowed_models` | string array | Model glob allowlist. Defaults to `["*"]`. |
| `allowed_modalities` | string array | Modality allowlist. Defaults to all modalities. |
| `allowed_toolsets` | string array | Toolset allowlist for tool execution. |
| `allowed_mcp_bindings` | string array | MCP binding allowlist. |
| `policy_models` | string array | Optional additional model policy gate. |
| `policy_modalities` | string array | Optional additional modality policy gate. |
| `policy_toolsets` | string array | Optional additional toolset policy gate. |
| `policy_mcp_bindings` | string array | Optional additional MCP binding policy gate. |
| `expires_at` | RFC3339 string or Unix seconds | Optional claim expiry. |

Example claims before base64url encoding:

```json
{
  "sub": "user_123",
  "project_id": "proj_123",
  "is_admin": false,
  "rate_limit": "60/min",
  "allowed_models": ["openai/*", "anthropic/*"],
  "allowed_modalities": ["chat", "embed"],
  "expires_at": "2026-04-25T20:30:00Z"
}
```

Control-plane endpoints (`/v1/projects`, `/v1/virtual_keys`, `/v1/policies`, `/v1/budgets`, `/v1/tools`, `/v1/toolsets`, `/v1/mcp/bindings`) require `control_plane.enabled: true` and admin access:

- in `auth.mode: virtual_keys`, use either the configured bootstrap admin key or a virtual key with `is_admin: true`
- in `auth.mode: external`, set `is_admin: true` in the signed claims from the trusted upstream platform
- in `auth.mode: multi-user`, use a legacy database API key with `is_admin: true`; prefer `virtual_keys` or `external` for new deployments

`/v1/keys` remains implemented as a compatibility facade. In `virtual_keys` mode it issues Polaris virtual keys using the older response shape. In `multi-user` mode it continues to manage the legacy `api_keys` rows.

Per `BLUEPRINT.md` §19:
- Keys are stored as SHA-256 hashes. Plaintext keys are never persisted.
- Logs contain `key_prefix` (first 8 characters) only. Never the full key.

### Error responses from the auth layer

| Condition | HTTP | `error.type` | `error.code` |
|---|---|---|---|
| Missing `Authorization` header | 401 | `authentication_error` | `missing_api_key` |
| Header present but key not found | 401 | `authentication_error` | `invalid_api_key` |
| Key found but revoked or expired | 401 | `authentication_error` | `key_revoked` / `key_expired` |
| Missing or invalid external auth headers | 401 | `authentication_error` | `missing_external_auth` / `invalid_external_auth_signature` / `external_auth_timestamp_expired` / `external_auth_claims_expired` |
| Key valid but not permitted to use the requested model | 403 | `permission_error` | `model_not_allowed` |
| Key valid but not permitted to use the requested modality/toolset/MCP binding | 403 | `permission_error` | `modality_not_allowed` / `toolset_not_allowed` / `mcp_binding_not_allowed` |
| Hard budget already exceeded for the current project | 429 | `budget_exceeded` | `budget_exceeded` |
| Admin endpoint called without admin key | 403 | `permission_error` | `admin_required` |

---

## 3. Model Naming

All model references use one of three shapes (`BLUEPRINT.md` §8.5):

- **Provider variant:** `openai/gpt-4o`, `anthropic/claude-sonnet-4-6`, `bytedance/doubao-seedance-2.0`
- **Model family:** `gpt-5.5`, `gpt-5.4-mini`, `claude-opus-4-7`, `amazon-nova-2-lite`
- **Alias:** static aliases from `routing.aliases`, selector aliases from `routing.selectors`, and family aliases from the embedded model matrix

Resolution order per request:
1. Exact configured aliases in `routing.aliases`
2. Exact provider variants in the registry
3. Family aliases from the embedded model matrix
4. Canonical family IDs
5. Selector aliases in `routing.selectors` for capability-driven routes
6. Check modality matches the endpoint (chat endpoint requires a chat model, etc.)
7. Check the authenticated key's `allowed_models` glob patterns permit the resolved provider variant
8. On any failure, return the appropriate error below

Exact provider variants are never re-routed. Family IDs and family aliases resolve deterministically to one currently enabled provider variant using the embedded family priority plus verification/status ordering.

### Request-level routing

Every model-taking JSON endpoint also accepts an optional `routing` object. Multipart endpoints accept the same value as a JSON-encoded `routing` form field.

```json
{
  "routing": {
    "providers": ["openai", "openrouter"],
    "exclude_providers": ["bedrock"],
    "capabilities": ["function_calling", "streaming"],
    "statuses": ["ga", "preview"],
    "verification_classes": ["strict", "opt_in"],
    "prefer": ["openai/gpt-5.4-mini"],
    "cost_tier": "balanced",
    "latency_tier": "fast"
  }
}
```

Rules:
- Exact `provider/model` requests always execute directly and ignore request-level routing hints.
- Family IDs, family aliases, and selector aliases merge request-level routing hints with static selector/config policy before resolution.
- Multipart endpoints use the same schema in the `routing` form field.

Routed responses may include:
- `X-Polaris-Resolved-Model`
- `X-Polaris-Resolved-Provider`
- `X-Polaris-Routing-Mode`

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
| `budget_exceeded` | 429 | Project hard budget has already been exceeded for this request window. |
| `provider_error` | 502 | Upstream provider returned an error or malformed response. |
| `timeout_error` | 504 | Upstream provider exceeded the configured timeout. |
| `internal_error` | 500 | Unhandled server-side error. The `X-Request-ID` should accompany any bug report. |

Provider-specific failures are normalized into the envelope above. Common stable `error.code` values include `provider_auth_failed`, `provider_rate_limit`, `quota_exceeded`, `provider_timeout`, `provider_transport_error`, `provider_server_error`, and `provider_invalid_response`. Polaris preserves a sanitized provider message in `error.message`, but raw provider payloads are not part of the public contract.

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
    "total_tokens": 17,
    "source": "provider_reported"
  }
}
```

`finish_reason` is one of `stop`, `length`, `tool_calls`, `content_filter`. When the assistant invokes a tool, `message.content` is `null` and `message.tool_calls` is present (same shape as OpenAI).
`usage.source` is `provider_reported` when the upstream provider returned usage directly, `estimated` when Polaris synthesized counts, and `unavailable` when no trustworthy count is available.

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

### `POST /v1/responses`

OpenAI Responses-style conversation surface over the same normalized chat runtime used by `/v1/chat/completions`. When the selected provider adapter implements native Responses passthrough, Polaris forwards the provider-native JSON/SSE shape; otherwise it renders a compatibility response from the shared chat result.

**Auth:** required.
**Modality:** `chat`.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias resolving to a chat model. |
| `input` | string \| object \| array | yes | String becomes one `user` message. Object/array forms use Polaris-normalized message objects with `role` + `content`. |
| `instructions` | string | no | Prepended as a `system` message before dispatch. |
| `temperature` | number | no | `0.0`–`2.0`. |
| `top_p` | number | no | `0.0`–`1.0`. |
| `max_output_tokens` | integer | no | Mapped to the shared internal `max_tokens` field. |
| `tools` | array | no | Same function-tool shape as `/v1/chat/completions`. |
| `tool_choice` | string \| object | no | Same semantics as `/v1/chat/completions`. |
| `text.format` | object | no | Same `response_format` payload supported by `/v1/chat/completions`. |
| `metadata` | object | no | String map preserved on the normalized request. |
| `stream` | boolean | no | If `true`, returns SSE event objects with `type` fields such as `response.created`, `response.output_text.delta`, and `response.completed`. |

#### Response body

```json
{
  "id": "resp_123",
  "object": "response",
  "created_at": 1744329600,
  "status": "completed",
  "model": "openai/gpt-4o",
  "output": [
    {
      "id": "msg_resp_123",
      "type": "message",
      "role": "assistant",
      "content": [
        {"type": "output_text", "text": "Hello from Polaris"}
      ]
    }
  ],
  "output_text": "Hello from Polaris",
  "usage": {
    "input_tokens": 12,
    "output_tokens": 5,
    "total_tokens": 17,
    "source": "provider_reported"
  }
}
```

Tool calls are emitted as additional `output[]` items with `type: "function_call"`, `name`, and `arguments`.
`usage.source` follows the same `provider_reported|estimated|unavailable` contract as `/v1/chat/completions`.

#### Streaming response (`stream: true`)

The response uses `text/event-stream`. Each `data:` frame contains a JSON event object:

```json
{"type":"response.created","response":{"id":"resp_123","status":"in_progress","model":"openai/gpt-4o"}}
{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"Hello"}
{"type":"response.completed","response":{"id":"resp_123","status":"completed","model":"openai/gpt-4o","output_text":"Hello","usage":{"input_tokens":12,"output_tokens":5,"total_tokens":17,"source":"provider_reported"}}}
```

The stream ends with `data: [DONE]`. Mid-stream errors use the standard Polaris `error` envelope and then `[DONE]`.

### `POST /v1/messages`

Anthropic Messages-style conversation surface over the same normalized chat runtime used by `/v1/chat/completions`. When the selected provider adapter implements native Messages passthrough, Polaris forwards the provider-native JSON/SSE shape; otherwise it renders a compatibility response from the shared chat result.

**Auth:** required.
**Modality:** `chat`.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias resolving to a chat model. |
| `system` | string | no | Prepended as a `system` message before dispatch. |
| `messages` | array | yes | Ordered Anthropic-style user/assistant messages. |
| `messages[].role` | string | yes | `user` or `assistant`. |
| `messages[].content` | string \| array | yes | String or block array. Supported blocks: `text`, `image`, `audio`, `tool_use`, `tool_result`. |
| `max_tokens` | integer | no | Shared generation ceiling. |
| `temperature` | number | no | `0.0`–`2.0`. |
| `top_p` | number | no | `0.0`–`1.0`. |
| `tools` | array | no | Anthropic-style tool definitions with `name`, `description`, and `input_schema`. |
| `tool_choice` | string \| object | no | Passed through the shared tool-choice translator. |
| `stop_sequences` | array | no | Mapped to shared stop sequences. |
| `metadata` | object | no | String map preserved on the normalized request. |
| `stream` | boolean | no | If `true`, returns Anthropic-style SSE event objects such as `message_start`, `content_block_delta`, `message_delta`, and `message_stop`. |

#### Response body

```json
{
  "id": "msg_123",
  "type": "message",
  "role": "assistant",
  "model": "anthropic/claude-sonnet-4-6",
  "content": [
    {"type": "text", "text": "Hello from Polaris"}
  ],
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 12,
    "output_tokens": 5,
    "source": "provider_reported"
  }
}
```

Assistant tool calls are emitted as `content[]` blocks with `type: "tool_use"`, `id`, `name`, and `input`.
`usage.source` follows the same `provider_reported|estimated|unavailable` contract as `/v1/chat/completions`.

#### Streaming response (`stream: true`)

The response uses `text/event-stream`. Each `data:` frame contains a JSON event object:

```json
{"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","content":[]}}
{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}
{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}
{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":12,"output_tokens":5,"source":"provider_reported"}}
{"type":"message_stop"}
```

The stream ends with `data: [DONE]`. Mid-stream errors use the standard Polaris `error` envelope and then `[DONE]`.

### `POST /v1/tokens/count`

Best-effort token counting utility for chat-style request payloads.

**Auth:** required.
**Modality:** none.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias resolving to a chat model. |
| `messages` | array | no | Polaris-normalized chat messages. Use this OR `input`. |
| `input` | string \| object \| array | no | Responses-style input. String becomes one `user` message. |
| `requested_interface` | string | no | `chat`, `responses`, or `messages`. Used only for attribution/notes in the current runtime. |
| `max_output_tokens` | integer | no | Optional override for output estimate. If omitted, Polaris uses model metadata when available. |
| `tool_context` | object | no | Reserved for future token-estimation extensions. |

If both `messages` and `input` are omitted, Polaris returns `400 missing_input`.

#### Response body

```json
{
  "model": "anthropic/claude-sonnet-4-6",
  "input_tokens": 29,
  "output_tokens_estimate": 8192,
  "source": "provider_reported",
  "notes": [
    "input tokens were returned by Anthropic's native token counting endpoint",
    "output_tokens_estimate remains a Polaris estimate derived from max_tokens limits"
  ]
}
```

Notes:
- `source` is `provider_reported` for chat models whose adapters implement native token counting today, currently Anthropic and Google Gemini.
- For other chat providers, Polaris falls back to `estimated`.
- `output_tokens_estimate` is always a Polaris estimate derived from explicit request limits or model metadata.
- `context_window` remains best-effort metadata only; Polaris does not hard-block requests from this endpoint.

---

## 7. Embeddings

Current implementation note: the endpoint is implemented end to end for OpenAI, Google, Amazon Bedrock Titan embeddings, and NVIDIA embedding models. It validates requests, resolves aliases, canonical families, and request-level routing hints, enforces allowed-model checks, records usage through the normal async logging path, and participates in the shared model and usage surfaces.

### `POST /v1/embeddings`

Generate embedding vectors for text inputs.

**Auth:** required.
**Modality:** `embed`.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias resolving to an embed model. |
| `routing` | object | no | Optional request-level routing hints. Exact `provider/model` values ignore it. |
| `input` | string \| array | yes | Either a single string or an array of strings. |
| `dimensions` | integer | no | Optional truncation (only supported by models that declare it, for example OpenAI, Google, Bedrock Titan v2, and NVIDIA embedding models that expose dynamic output dimensions). |
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
    "total_tokens": 8,
    "source": "provider_reported"
  }
}
```

When `encoding_format: base64`, `embedding` is a base64-encoded string of little-endian float32 values.
`usage.source` follows the same `provider_reported|estimated|unavailable` contract as the chat surfaces.

#### Errors

Same set as chat. Streaming-mode errors are emitted in-band as SSE `error` envelopes after headers flush.

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

## 7A. Translations

Current implementation note: the translation route is implemented end to end for ByteDance machine translation models on the new OpenSpeech API-key path. It validates requests, resolves aliases and canonical models, enforces allowed-model checks, and records usage through the normal async logging path.

### `POST /v1/translations`

Translate one or more text inputs into a target language.

**Auth:** required.
**Modality:** `translation`.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias resolving to a translation model. |
| `input` | string \| array | yes | Either a single string or an array of strings. ByteDance currently supports up to 16 entries per request. |
| `target_language` | string | yes | Provider language code such as `en`, `es`, `fr`, `ja`, or `ko`. |
| `source_language` | string | no | Optional provider language code. When omitted, the provider may auto-detect the source language. |
| `glossary` | object | no | Optional inline term mapping. Polaris forwards this to provider glossary support when available. |

#### Response body

```json
{
  "object": "translation.list",
  "model": "bytedance/doubao-translation-2.0",
  "translations": [
    {
      "index": 0,
      "text": "Hola, Polaris.",
      "detected_source_language": "en"
    }
  ],
  "usage": {
    "prompt_tokens": 11,
    "completion_tokens": 5,
    "total_tokens": 16,
    "source": "provider_reported"
  }
}
```

#### Errors

Same set as chat. Provider-side translation token-limit failures are normalized as request validation errors rather than leaking raw vendor codes.

#### curl

```bash
curl http://localhost:8080/v1/translations \
  -H "Authorization: Bearer $POLARIS_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "bytedance/doubao-translation-2.0",
    "input": ["Hello, Polaris."],
    "target_language": "es"
  }'
```

---

## 8. Images

Current implementation note: the image routes are implemented end to end for OpenAI, Google, ByteDance, and Qwen image models. They validate requests, resolve aliases and canonical models, enforce allowed-model checks and image capabilities, and record usage through the normal async logging path.

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

When a provider returns inline image bytes rather than a hosted asset URL, Polaris normalizes `response_format: "url"` as a `data:` URL. In the current build this applies to Google image responses and OpenAI GPT Image responses. When a provider returns only hosted URLs but the caller requests `b64_json`, Polaris may fetch the returned asset and re-encode it. In the current build this applies to Qwen image responses.

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
  -F "model=openai/gpt-image-2" \
  -F "image=@./original.png" \
  -F "mask=@./mask.png" \
  -F "prompt=Replace the sky with a starfield"
```

---

## 9. Video

Current implementation note: video generation is implemented as an async submit/poll/content-download flow. Polaris currently supports ByteDance Seedance, OpenAI Sora, Google Vertex Veo, and Replicate Predictions video models behind the shared `POST / GET / DELETE / GET content` surface.

### `POST /v1/video/generations`

Submit a video generation job.

**Auth:** required.
**Modality:** `video`.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias resolving to a video model. |
| `prompt` | string | yes | Scene description. |
| `duration` | integer | no | Seconds. Provider default if omitted. If the model advertises `allowed_durations`, the value must match one of them. |
| `aspect_ratio` | string | no | e.g. `16:9`, `9:16`. If the model advertises `aspect_ratios`, the value must match one of them. |
| `resolution` | string | no | e.g. `720p`. Must be listed in the model's `resolutions`. |
| `first_frame` | string | no | URL or base64 image anchoring the opening frame. Requires `image_to_video` capability. |
| `last_frame` | string | no | URL or base64 image anchoring the ending frame. Requires `first_frame` and `last_frame` capability. |
| `reference_images` | array | no | Additional reference images (style/subject anchors). Requires `reference_images`. Cannot be combined with `first_frame`/`last_frame`. |
| `reference_videos` | array | no | Additional reference videos. Requires `video_input`. Cannot be combined with `first_frame`/`last_frame`. |
| `audio` | string | no | Synced input audio. Requires `audio_input` and at least one `reference_images` or `reference_videos` item. Cannot be combined with `first_frame`/`last_frame`. |
| `with_audio` | boolean | no | Generate native audio track (requires `native_audio` capability). |

Provider notes:
- ByteDance Seedance supports the full shared request surface above.
- OpenAI Sora currently supports `prompt`, `first_frame`, `duration`, `aspect_ratio`, `resolution`, and `with_audio`.
- Google Vertex Veo currently supports `prompt`, `first_frame`, `last_frame`, `duration`, `aspect_ratio`, `resolution`, and `with_audio`.
- Replicate Predictions currently supports the async text-to-video subset exposed by the configured Replicate model and normalizes completed output URLs through the Polaris content endpoint.

`first_frame` only, `first_frame` + `last_frame`, and multimodal reference mode (`reference_images` / `reference_videos` / input `audio`) are treated as mutually exclusive scenes by the Seedance provider. Polaris validates that upfront.

#### Response body (job submitted)

```json
{
  "job_id": "vid_01HXYZ...",
  "status": "queued",
  "estimated_time": 45,
  "model": "bytedance/doubao-seedance-2.0"
}
```

`job_id` is an opaque Polaris-issued signed token, not the raw provider task id.
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
  "created_at": 1712697600,
  "completed_at": 1712697610,
  "expires_at": 1712701200,
  "result": {
    "video_url": "https://...",
    "audio_url": "https://...",
    "download_url": "https://gateway.example/v1/video/generations/vid_01HXYZ.../content",
    "content_type": "video/mp4",
    "duration": 8,
    "width": 1920,
    "height": 1080
  },
  "error": null
}
```

- `progress` is `0.0`–`1.0`.
- `created_at`, `completed_at`, and `expires_at` are Unix timestamps in seconds when the provider exposes them.
- `result` is populated only when `status == "completed"`.
- `download_url` is the Polaris-owned content endpoint for the primary rendered video asset.
- `error` is populated only when `status == "failed"` and carries a nested `{type, message, code}` envelope matching §4.

### `GET /v1/video/generations/:id/content`

Download the primary rendered video bytes for a completed job.

Returns `200 OK` with a binary body and `Content-Type` matching the normalized video asset, typically `video/mp4`.

### `DELETE /v1/video/generations/:id`

Cancel a running job. Returns `204 No Content` on success. Jobs already in `completed` or `failed` state return `409 invalid_request_error / job_immutable`. Models that Polaris marks as non-cancelable, such as OpenAI Sora in the current implementation, return `409 invalid_request_error / job_not_cancelable`.

The `:id` path parameter must be the opaque `job_id` returned by Polaris during submit.

#### Errors

| HTTP | `type` | Cause |
|---|---|---|
| 400 | `invalid_request_error` | Missing `prompt`, conflicting parity inputs, `last_frame` without `first_frame`, empty `reference_videos`, `audio` without reference media, invalid `duration`, unsupported `aspect_ratio`, or unsupported `resolution`. |
| 400 | `capability_not_supported` | Request uses `first_frame`/`reference_*`/`with_audio` but the model lacks the capability. |
| 404 | `model_not_found` | Unknown model or alias. |
| 404 | `invalid_request_error / job_not_found` | Polling or cancelling an unknown job id. |
| 409 | `invalid_request_error / job_not_ready` | Download requested before the job completed. |
| 409 | `invalid_request_error / job_immutable` | Cancelling a terminal job. |
| 409 | `invalid_request_error / job_not_cancelable` | Cancelling a non-cancelable model. |
| 410 | `invalid_request_error / asset_expired` | Download requested after the provider asset expired. |
| 429/502/504 | — | Standard set. |

#### curl

```bash
# Submit
curl http://localhost:8080/v1/video/generations \
  -H "Authorization: Bearer $POLARIS_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "bytedance/doubao-seedance-2.0",
    "prompt": "A drone flyover of a snowy mountain range at sunrise",
    "duration": 8,
    "resolution": "720p",
    "with_audio": true
  }'

# Poll
curl http://localhost:8080/v1/video/generations/vid_01HXYZ... \
  -H "Authorization: Bearer $POLARIS_KEY"

# Download content
curl http://localhost:8080/v1/video/generations/vid_01HXYZ.../content \
  -H "Authorization: Bearer $POLARIS_KEY" \
  --output out.mp4

# Cancel
curl -X DELETE http://localhost:8080/v1/video/generations/vid_01HXYZ... \
  -H "Authorization: Bearer $POLARIS_KEY"
```

---

## 10. Music

Current implementation note: music is a first-class Polaris modality. The shipped Phase 5A surface is provider-neutral but capability-gated. For `v2.1.0`, MiniMax is the release-blocking music provider and backs generation, cover edits, and lyrics. ElevenLabs backs generation, streaming generation, stems, and composition plans through the same API shape, but that provider path is currently treated as preview until it is explicitly opted into live smoke. Async music jobs are Polaris-managed and require a configured cache backend. `sync` remains the default request mode, but long-running music jobs, especially MiniMax generation, should use `mode: "async"`.

### `POST /v1/music/generations`

Generate music.

**Auth:** required.
**Modality:** `music`.

#### Request body (JSON)

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | Must resolve to a `modality: music` model with `music_generation`. |
| `mode` | string | no | `sync` (default), `async`, or `stream`. `stream` requires `music_streaming`. |
| `prompt` | string | no | Description of the desired track. |
| `lyrics` | string | no | Optional lyrics seed or full lyric body. |
| `plan` | object | no | Optional provider-neutral composition plan JSON. |
| `duration_ms` | integer | no | Must fall within the selected model's advertised duration range when configured. |
| `instrumental` | boolean | no | Requires `instrumental` capability when `true`. |
| `seed` | integer | no | Optional deterministic seed hint. |
| `output_format` | string | no | Model-gated audio format such as `mp3`, `wav`, `flac`, `pcm`, or `opus`. |
| `sample_rate_hz` | integer | no | Model-gated sample rate hint. |
| `bitrate` | integer | no | Optional bitrate hint. |
| `store_for_editing` | boolean | no | Provider hint for later edit/inpaint workflows where supported. |
| `sign_with_c2pa` | boolean | no | Provider hint for provenance signing where supported. |

`sync` returns raw audio bytes. `stream` returns chunked audio bytes. `async` returns a signed Polaris `job_id`. For MiniMax and other long-running jobs, `async` is the recommended production path.

### `POST /v1/music/edits`

Edit music with one of the unified operations: `remix`, `extend`, `cover`, or `inpaint`.

Exactly one source is required:

- `source_job_id`
- `source_audio`
- uploaded multipart `file`

The request supports both JSON and `multipart/form-data`. `mode` defaults to `sync`; `stream` requires `music_streaming`; `async` returns a signed `job_id`. Capability gating is operation-aware:

- `cover` requires `music_cover`
- `extend` requires `music_extension`
- `inpaint` requires `music_inpainting`
- all edits require `music_editing`

### `POST /v1/music/stems`

Separate stems from a prior Polaris music job, an uploaded file, or a provider-supported `source_audio` reference.

**Modes:** `sync` (default) or `async`.

`sync` returns a ZIP archive. `async` returns a signed `job_id`.

### `POST /v1/music/lyrics`

Generate lyrics JSON.

```json
{
  "title": "Skyline",
  "style_tags": "pop",
  "lyrics": "..."
}
```

### `POST /v1/music/plans`

Generate or refine a composition plan JSON document.

```json
{
  "plan": {
    "sections": [
      {"name": "intro"}
    ]
  }
}
```

### `GET /v1/music/jobs/:id`

Poll an async music job. Response shape:

```json
{
  "job_id": "mus_01HXYZ...",
  "status": "completed",
  "model": "minimax/music-2.6",
  "operation": "generate",
  "created_at": 1712697600,
  "completed_at": 1712697610,
  "expires_at": 1712784000,
  "result": {
    "download_url": "https://gateway.example/v1/music/jobs/mus_01HXYZ.../content",
    "content_type": "audio/mpeg",
    "filename": "music.mp3",
    "duration_ms": 120000,
    "sample_rate_hz": 44100,
    "bitrate": 128,
    "size_bytes": 1048576
  }
}
```

### `GET /v1/music/jobs/:id/content`

Download the primary rendered music bytes or stems ZIP for a completed async job.

- `409 job_not_ready` when the job is not complete
- `410 asset_expired` when the cached job asset has expired

### `DELETE /v1/music/jobs/:id`

Cancel an async Polaris-managed music job. Completed or already terminal jobs return `204 No Content`.

### Music errors

| HTTP | `type` | Cause |
|---|---|---|
| 400 | `invalid_request_error` | Missing `model`, missing prompt/lyrics/plan, invalid `mode`, invalid `operation`, invalid duration/sample-rate/bitrate, or invalid source selection. |
| 400 | `capability_not_supported` | Model lacks `music_generation`, `music_streaming`, `music_editing`, `music_cover`, `music_extension`, `music_inpainting`, `music_stems`, `lyrics_generation`, or `composition_plans`. |
| 403 | `permission_error` | Job ownership mismatch on poll/cancel/content. |
| 404 | `invalid_request_error` | Unknown `job_id`. |
| 409 | `invalid_request_error` | Async jobs unavailable without a cache backend, or source/content job not ready. |
| 504 | `timeout_error / provider_timeout` | Upstream music generation timed out. Retry with `mode=async` for long-running jobs or increase the provider timeout. |
| 410 | `invalid_request_error` | Music asset expired. |

## 11. Voice — TTS & STT

Current implementation note: the voice surface is provider-backed. `POST /v1/audio/speech` is implemented end to end for OpenAI and ByteDance TTS. `POST /v1/audio/transcriptions` is implemented end to end for OpenAI and ByteDance STT. `POST /v1/audio/transcriptions/stream` plus `GET /v1/audio/transcriptions/stream/:id/ws` are implemented for ByteDance streaming STT 2.0. `POST /v1/audio/interpreting/sessions` plus `GET /v1/audio/interpreting/sessions/:id/ws` are implemented for ByteDance simultaneous interpretation 2.0. `POST /v1/audio/notes` plus `GET /v1/audio/notes/:id` are implemented for ByteDance notes. `POST /v1/audio/podcasts` plus `GET /v1/audio/podcasts/:id` and `GET /v1/audio/podcasts/:id/content` are implemented for ByteDance podcast generation. ByteDance TTS uses the new-console V3 TTS 2.0 SSE surface with provider-managed 2.0 speaker IDs. ByteDance STT uses the direct-upload file-recognition 2.0 compatibility endpoint and Polaris synthesizes `text`, `srt`, and `vtt` responses from the returned utterance timestamps. ByteDance streaming STT and interpreting keep the public websocket contract provider-neutral and translate the provider binary websocket protocol into JSON transcript or interpretation events.

### `GET /v1/voices`

List available voices either from Polaris model configuration or from a provider-backed catalog.

**Auth:** required.
**Modality:** none.

#### Query parameters

| Field | Type | Required | Notes |
|---|---|---|---|
| `scope` | string | no | `config` (default) or `provider`. |
| `provider` | string | no | Required when `scope=provider` unless `model` is supplied and resolves to a provider. |
| `model` | string | no | Optional `provider/model` or alias. With `scope=config`, filters configured voices to that model. With `scope=provider`, constrains provider-backed listing to the selected model's configured voice IDs when applicable. |
| `type` | string | no | `builtin` (default), `custom`, or `all`. ByteDance currently supports all three modes through the built-in catalog plus custom voice assets. |
| `state` | string | no | Reserved for future voice-asset filtering. |
| `limit` | integer | no | Positive integer limit applied after normalization. |
| `include_archived` | boolean | no | When `true`, include Polaris-local archived provider voices and mark them with `metadata.archived=true`. |

#### Response body

```json
{
  "object": "list",
  "scope": "provider",
  "provider": "bytedance",
  "data": [
    {
      "id": "zh_female_vv_uranus_bigtts",
      "provider": "bytedance",
      "type": "builtin",
      "name": "Uranus",
      "gender": "female",
      "age": "adult",
      "categories": ["Narration"],
      "models": ["bytedance/doubao-tts-2.0"],
      "preview_url": "https://example.com/demo.mp3",
      "preview_text": "Hello from Uranus.",
      "emotions": [
        {
          "name": "General",
          "type": "general",
          "preview_url": "https://example.com/demo.mp3",
          "preview_text": "Hello from Uranus."
        }
      ]
    }
  ]
}
```

`scope=config` returns voices declared directly on model config blocks and normalizes them into `type: "configured"` entries. `scope=provider` returns provider-backed metadata when an adapter is available. In the current runtime, provider-backed voice listing is implemented for ByteDance built-in and custom voices through the OpenSpeech control plane and the newer voice-asset APIs.

#### Errors

| HTTP | `type` | Cause |
|---|---|---|
| 400 | `invalid_request_error` | Invalid `scope`, `type`, `limit`, or missing `provider` when `scope=provider` and `model` is not set. |
| 400 | `invalid_request_error` | `provider_model_mismatch` when `provider` does not match the selected model. |
| 400 | `invalid_request_error` | `provider_catalog_unavailable` when the provider has no voice catalog adapter configured. |
| 404 | `invalid_request_error` | Unknown `model` or alias. |
| 502 | `provider_error` | Upstream provider catalog lookup failed. |

#### curl

```bash
# Config-backed listing
curl "http://localhost:8080/v1/voices?scope=config&model=bytedance-tts" \
  -H "Authorization: Bearer $POLARIS_KEY"

# Provider-backed ByteDance listing
curl "http://localhost:8080/v1/voices?scope=provider&provider=bytedance&limit=5" \
  -H "Authorization: Bearer $POLARIS_KEY"
```

### `GET /v1/voices/:id`

Fetch one normalized voice resource.

Query parameters:

| Field | Type | Required | Notes |
|---|---|---|---|
| `scope` | string | no | `provider` (default) or `config`. |
| `provider` | string | no | Required when `scope=provider` and `model` is not set. |
| `model` | string | no | Optional `provider/model` or alias. |
| `type` | string | no | `builtin`, `custom`, or `all` when `scope=provider`. Defaults to `all`. |
| `include_archived` | boolean | no | When `true`, return Polaris-local archived provider voices instead of hiding them. |

Response body: one `VoiceItem`.

### `POST /v1/voices/:id/archive`

Archive or hide a provider-backed voice locally inside Polaris without deleting it upstream.

Current runtime note:

- This is the supported ByteDance close-down path when you want a custom or built-in provider voice to disappear from normal `/v1/voices` listings without pretending the provider deleted it.
- Archived voices are hidden by default from `GET /v1/voices` and `GET /v1/voices/:id`.
- Add `include_archived=true` to surface archived entries again.

Query parameters:

| Field | Type | Required | Notes |
|---|---|---|---|
| `provider` | string | no | Required when `model` is not set. |
| `model` | string | no | Optional `provider/model` or alias. |
| `type` | string | no | `builtin`, `custom`, or `all`. Defaults to `all`. |

Response: `204 No Content`

### `POST /v1/voices/:id/unarchive`

Remove a Polaris-local archive marker and restore the voice to normal listings.

Query parameters:

| Field | Type | Required | Notes |
|---|---|---|---|
| `provider` | string | no | Required when `model` is not set. |
| `model` | string | no | Optional `provider/model` or alias. |

Response: `204 No Content`

### `POST /v1/voices/clones`

Create a provider-backed custom voice by cloning a reference recording.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | Must declare `voice_cloning`. |
| `audio` | string | yes | Base64-encoded source audio. |
| `audio_format` | string | no | Provider hint such as `wav`, `mp3`, or `m4a`. |
| `language` | string | no | Optional locale hint. |
| `prompt_text` | string | no | Optional transcript or prompt text. |
| `preview_text` | string | no | Optional preview text stored with the voice. |
| `denoise` | boolean | no | Provider denoise hint. |
| `check_prompt_text_quality` | boolean | no | Provider quality check hint. |
| `check_audio_quality` | boolean | no | Provider quality check hint. |
| `enable_source_separation` | boolean | no | Provider source-separation hint. |
| `denoise_model` | string | no | Provider-specific denoise model selection. |

Response body: one `VoiceItem`. Typical initial states are `draft` or `training`.

### `POST /v1/voices/designs`

Create a provider-backed custom voice from a structured text description.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | Must declare `voice_design`. |
| `text` | string | yes | Voice style specification. |
| `prompt_text` | string | no | Optional preview or conditioning text. |
| `prompt_image_url` | string | no | Optional image URL when the provider supports image-guided design. |

Response body: one `VoiceItem`.

### `POST /v1/voices/:id/retrain`

Retrain an existing custom voice with another reference recording.

Request body: same schema as `POST /v1/voices/clones`, with `voice_id` taken from the path.

### `POST /v1/voices/:id/activate`

Activate or finalize a completed custom voice.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | Voice-capable model or alias. |
| `provider` | string | no | Optional provider override when `model` is omitted from query resolution. |

Response body: one `VoiceItem`.

### `DELETE /v1/voices/:id`

Delete a provider-backed custom voice.

Current runtime note:

- ByteDance currently returns `400 capability_not_supported / voice_delete_not_supported` because the provider delete path is not exposed through Polaris yet.
- Use `POST /v1/voices/:id/archive` if you need a truthful Polaris-local hide path instead of provider deletion.

### `POST /v1/audio/notes`

Submit an async audio-note job.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | Must declare `audio_notes`. |
| `source_url` | string | yes | Provider-fetchable audio URL. |
| `file_type` | string | no | Optional source type hint such as `wav` or `mp3`. |
| `language` | string | no | Optional source language hint. |
| `include_summary` | boolean | no | Include a summary in the final result. |
| `include_chapters` | boolean | no | Include chapter segmentation. |
| `include_action_items` | boolean | no | Include action-item extraction. |
| `include_qa_pairs` | boolean | no | Include Q&A extraction. |
| `target_language` | string | no | Optional translation target for the final output. |

Response body:

```json
{
  "id": "not_01HXYZ...",
  "object": "audio.note",
  "model": "bytedance/doubao-notes-2.0",
  "status": "queued"
}
```

### `GET /v1/audio/notes/:id`

Poll an async note job. Terminal success returns:

```json
{
  "id": "not_01HXYZ...",
  "object": "audio.note",
  "model": "bytedance/doubao-notes-2.0",
  "status": "completed",
  "result": {
    "transcript": "...",
    "summary": "...",
    "chapters": [],
    "action_items": [],
    "qa_pairs": [],
    "translation": "...",
    "metadata": {}
  }
}
```

Current runtime note:

- `DELETE /v1/audio/notes/:id` is implemented, but ByteDance currently returns `400 capability_not_supported / notes_delete_not_supported`.

### `POST /v1/audio/podcasts`

Submit an async podcast generation job.

#### Request body

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | Must declare `podcast_generation`. |
| `segments` | array | yes | Ordered script segments. At least one element. |
| `segments[].speaker` | string | yes | Logical speaker label. |
| `segments[].voice` | string | no | Optional concrete voice id. If omitted, the provider may use `speaker`. |
| `segments[].text` | string | yes | Segment text. |
| `output_format` | string | no | `mp3`, `ogg_opus`, `pcm`, or `aac`. |
| `sample_rate_hz` | integer | no | `16000`, `24000`, or `48000`. |
| `use_head_music` | boolean | no | Provider hint for intro music. |

Response body:

```json
{
  "id": "pod_01HXYZ...",
  "object": "audio.podcast",
  "model": "bytedance/doubao-podcast-2.0",
  "status": "queued"
}
```

### `GET /v1/audio/podcasts/:id`

Poll an async podcast job. Terminal success returns:

```json
{
  "id": "pod_01HXYZ...",
  "object": "audio.podcast",
  "model": "bytedance/doubao-podcast-2.0",
  "status": "completed",
  "result": {
    "content_type": "audio/mpeg",
    "usage": {
      "prompt_tokens": 100,
      "completion_tokens": 200,
      "total_tokens": 300,
      "source": "provider_reported"
    },
    "metadata": {
      "audio_url": "https://..."
    }
  }
}
```

### `GET /v1/audio/podcasts/:id/content`

Download the completed podcast bytes.

- `409 job_not_ready` when the async job is not terminal
- `400 provider_invalid_response` when the provider returned no downloadable asset

### `DELETE /v1/audio/podcasts/:id`

Cancel an async podcast job. Completed or already terminal jobs return `204 No Content`.

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
| `response_format` | string | no | `mp3` (default), `opus`, `aac`, `flac`, `wav`, `pcm`. Provider support varies; the current ByteDance TTS adapter supports `mp3`, `opus`, and `pcm`. |
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

### Streaming transcription sessions

Create an ephemeral streaming transcription session and then connect to it over WebSocket.

Bootstrap endpoint:

- `POST /v1/audio/transcriptions/stream`

WebSocket transport:

- `GET /v1/audio/transcriptions/stream/:id/ws`

`POST /v1/audio/transcriptions/stream` uses the normal Polaris API key. The WebSocket connect step uses the returned `client_secret`:

```text
Authorization: Bearer <client_secret>
```

Current runtime limits:

- `input_audio_format` must be `pcm16`
- `sample_rate_hz` must be `16000`
- ByteDance currently backs the shipped implementation
- ByteDance streaming transcription uses `providers.bytedance.app_id` plus `providers.bytedance.speech_access_token`

Session response:

```json
{
  "id": "sttsess_01HXYZ...",
  "object": "audio.transcription.session",
  "model": "bytedance/doubao-streaming-asr-2.0",
  "expires_at": 1712699999,
  "websocket_url": "wss://gateway.example/v1/audio/transcriptions/stream/sttsess_01HXYZ.../ws",
  "client_secret": "sttsec_01HXYZ..."
}
```

#### Bootstrap request fields

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias. Must resolve to a `modality: voice` model with `streaming` capability. |
| `input_audio_format` | string | no | Must be `pcm16`. Defaults to `pcm16`. |
| `sample_rate_hz` | integer | no | Must be `16000`. Defaults to `16000`. |
| `language` | string | no | Provider-dependent. The current ByteDance `bigmodel_async` path does not accept `language`; only `bigmodel_nostream` does. |
| `interim_results` | boolean | no | Defaults to `true`. |
| `return_utterances` | boolean | no | Defaults to `true`. |

Client event types:

- `session.update`
- `input_audio.append`
- `input_audio.commit`
- `session.close`

Server event types:

- `session.created`
- `session.updated`
- `input_audio.committed`
- `transcript.delta`
- `transcript.segment`
- `transcript.completed`
- `error`

Wire contract:

- provider-neutral JSON events
- audio payloads encoded as base64 strings
- create-then-connect session flow
- stateless signed session ids and client secrets

Streaming event payload notes:

- `transcript.delta.text` carries the incremental text since the last provider update
- `transcript.segment.segment` carries stable utterance segments with `final: true`
- `transcript.completed.transcript` uses the same normalized transcript envelope as `POST /v1/audio/transcriptions`

Current runtime note:

- `/v1/models` exposes streaming transcription models with `capabilities: ["streaming"]`
- the public Go SDK exposes `CreateStreamingTranscriptionSession` and `DialStreamingTranscriptionSession`
- the current ByteDance adapter targets `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async` and maps configured models onto the current `volc.seedasr.sauc.duration` / `volc.seedasr.sauc.concurrent` resource ids

#### curl

```bash
curl http://localhost:8080/v1/audio/transcriptions/stream \
  -H "Authorization: Bearer $POLARIS_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "bytedance/doubao-streaming-asr-2.0",
    "sample_rate_hz": 16000
  }'
```

### Simultaneous interpretation sessions

Create an ephemeral interpretation session and then connect to it over WebSocket.

Bootstrap endpoint:

- `POST /v1/audio/interpreting/sessions`

WebSocket transport:

- `GET /v1/audio/interpreting/sessions/:id/ws`

`POST /v1/audio/interpreting/sessions` uses the normal Polaris API key. The WebSocket connect step uses the returned `client_secret`:

```text
Authorization: Bearer <client_secret>
```

Current runtime limits:

- `mode` must be `speech_to_speech` or `speech_to_text`
- current shipped provider path is ByteDance simultaneous interpretation 2.0
- ByteDance interpreting uses `providers.bytedance.app_id` plus `providers.bytedance.speech_access_token`
- ByteDance interpreting input must be `16000 Hz`, mono, `pcm16` or `wav`
- ByteDance interpreting internally maps the source stream onto the provider AST requirement `wav + raw + 16000 Hz`
- `speech_to_text` returns translated text only
- `speech_to_speech` additionally returns translated audio chunks

Session response:

```json
{
  "id": "intsess_01HXYZ...",
  "object": "audio.interpreting.session",
  "model": "bytedance/doubao-interpreting-2.0",
  "expires_at": 1712699999,
  "websocket_url": "wss://gateway.example/v1/audio/interpreting/sessions/intsess_01HXYZ.../ws",
  "client_secret": "intsec_01HXYZ..."
}
```

#### Bootstrap request fields

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias. Must resolve to a `modality: interpreting` model. |
| `mode` | string | no | `speech_to_speech` (default) or `speech_to_text`. |
| `source_language` | string | yes | Provider language code such as `en` or `zh`. |
| `target_language` | string | yes | Provider language code such as `en` or `zh`. |
| `input_audio_format` | string | no | `pcm16` (default) or `wav`. |
| `input_sample_rate_hz` | integer | no | Must be `16000`. Defaults to `16000`. |
| `output_audio_format` | string | no | `pcm16` or `ogg_opus`. Used only for `speech_to_speech`. |
| `output_sample_rate_hz` | integer | no | `16000` for `pcm16`, `48000` for `ogg_opus`. |
| `voice` | string | no | Optional provider voice or speaker id for `speech_to_speech`. |
| `denoise` | boolean | no | Provider hint for input denoise. |
| `glossary` | array | no | Optional list of `{source,target}` glossary entries. |

Client event types:

- `session.update`
- `input_audio.append`
- `input_audio.commit`
- `session.close`

Server event types:

- `session.created`
- `session.updated`
- `input_audio.committed`
- `input_audio.muted`
- `source_transcript.delta`
- `source_transcript.segment`
- `translation.delta`
- `translation.segment`
- `response.audio.delta`
- `response.audio.done`
- `response.completed`
- `error`

Current runtime note:

- `/v1/models` exposes interpreting models as `modality: interpreting`
- the public Go SDK exposes `CreateInterpretingSession` and `DialInterpretingSession`
- the current ByteDance adapter targets `wss://openspeech.bytedance.com/api/v4/ast/v2/translate`
- the current ByteDance adapter uses resource id `volc.service_type.10053`
- live smoke is validated with chunked 80 ms PCM input, matching the provider guidance

#### curl

```bash
curl http://localhost:8080/v1/audio/interpreting/sessions \
  -H "Authorization: Bearer $POLARIS_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "bytedance/doubao-interpreting-2.0",
    "mode": "speech_to_text",
    "source_language": "en",
    "target_language": "zh",
    "input_audio_format": "pcm16",
    "input_sample_rate_hz": 16000
  }'
```

### Full-Duplex Audio Sessions

Create an ephemeral full-duplex audio session and then connect to it over WebSocket.

Bootstrap endpoint:

- `POST /v1/audio/sessions`

WebSocket transport:

- `GET /v1/audio/sessions/:id/ws`

`POST /v1/audio/sessions` uses the normal Polaris API key. The WebSocket connect step uses the returned `client_secret`:

```text
Authorization: Bearer <client_secret>
```

Current runtime limits:

- `input_audio_format` and `output_audio_format` must be `pcm16`
- `sample_rate_hz` must be `16000`
- OpenAI native Realtime audio models support `manual` and `server_vad`
- ByteDance native realtime audio models support `manual` and `server_vad`
- ByteDance native realtime sessions run over the provider websocket dialogue transport and keep the public contract normalized to `pcm16` / `16000 Hz`
- The default auth path follows the current official realtime docs and uses `providers.bytedance.app_id` plus `providers.bytedance.speech_access_token`
- Accounts that expose realtime API-key auth can opt into `realtime_session.auth: api_key` with `providers.bytedance.speech_api_key`
- legacy provider-backed cascaded audio sessions are still supported through `audio_pipeline` models and are compatibility sessions, not native provider realtime sessions

Session response:

```json
{
  "id": "audsess_01HXYZ...",
  "object": "audio.session",
  "model": "openai/gpt-4o-audio",
  "expires_at": 1712699999,
  "websocket_url": "wss://gateway.example/v1/audio/sessions/audsess_01HXYZ.../ws",
  "client_secret": "audsec_01HXYZ..."
}
```

#### Bootstrap request fields

| Field | Type | Required | Notes |
|---|---|---|---|
| `model` | string | yes | `provider/model` or alias. Must resolve to a `modality: audio` model with `audio_input` and `audio_output`. |
| `voice` | string | no | Defaults to the first configured voice for the audio model when available. |
| `instructions` | string | no | System instructions applied to subsequent turns. |
| `input_audio_format` | string | no | Must be `pcm16`. Defaults to `pcm16`. |
| `output_audio_format` | string | no | Must be `pcm16`. Defaults to `pcm16`. |
| `sample_rate_hz` | integer | no | Must be `16000`. Defaults to `16000`. |
| `turn_detection` | object | no | `{"mode":"manual"}` or provider-supported modes such as OpenAI `server_vad`. |

Client event types:

- `session.update`
- `input_audio.append`
- `input_audio.commit`
- `input_text`
- `response.create`
- `response.cancel`
- `session.close`

Server event types:

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

Wire contract:

- provider-neutral JSON events
- audio payloads encoded as base64 strings
- create-then-connect session flow
- stateless signed session ids and client secrets

Current runtime note:

- `input_text` plus `response.create` is valid even without committed audio
- `server_vad` in the current OpenAI-backed slice auto-starts a response after `input_audio.commit`
- `response.completed.usage.source` follows the same `provider_reported|estimated|unavailable` contract as the text and embedding surfaces
- `/v1/models` exposes audio models, including `voices` and `session_ttl` when configured
- the public Go SDK exposes `CreateAudioSession` and `DialAudioSession`

---

## 12. Models

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
      "kind": "provider_variant",
      "provider": "openai",
      "provider_variant": "openai/gpt-4o",
      "family_id": "gpt-4o",
      "family_display_name": "GPT-4o",
      "modality": "chat",
      "capabilities": ["vision", "function_calling", "streaming", "audio_input", "audio_output"],
      "context_window": 128000,
      "max_output_tokens": 16384
    },
    {
      "id": "gpt-5.5",
      "object": "model",
      "kind": "family",
      "provider": "openai",
      "provider_variant": "openai/gpt-5.5",
      "display_name": "GPT-5.5",
      "family_id": "gpt-5.5",
      "family_display_name": "GPT-5.5",
      "modality": "chat",
      "capabilities": ["streaming", "function_calling", "vision", "json_mode", "reasoning"],
      "resolves_to": "openai/gpt-5.5"
    },
    {
      "id": "openai/text-embedding-3-small",
      "object": "model",
      "kind": "provider_variant",
      "provider": "openai",
      "provider_variant": "openai/text-embedding-3-small",
      "family_id": "text-embedding-3-small",
      "family_display_name": "text-embedding-3-small",
      "modality": "embed",
      "dimensions": 1536
    },
    {
      "id": "openai/gpt-image-2",
      "object": "model",
      "provider": "openai",
      "modality": "image",
      "capabilities": ["generation", "editing"],
      "output_formats": ["png", "jpeg", "webp"]
    },
    {
      "id": "openai/tts-1",
      "object": "model",
      "provider": "openai",
      "modality": "voice",
      "capabilities": ["tts"],
      "voices": ["alloy", "echo", "fable", "onyx", "nova", "shimmer"]
    },
    {
      "id": "openai/whisper-1",
      "object": "model",
      "provider": "openai",
      "modality": "voice",
      "capabilities": ["stt"],
      "formats": ["mp3", "mp4", "mpeg", "mpga", "m4a", "wav", "webm"]
    }
  ]
}
```

The field set for each entry depends on the modality and mirrors the model's config block in `polaris.yaml`. Phase 3 modality-specific metadata includes `dimensions` for embeddings, `output_formats` for images, `voices` for TTS models, and `formats` for STT models. Video models may also expose `allowed_durations`, `aspect_ratios`, `resolutions`, `max_duration`, and `cancelable`. Music models may expose `output_formats`, `min_duration_ms`, `max_duration_ms`, and `sample_rates_hz`. Audio-session models may expose `voices` and `session_ttl`.

Family-aware metadata:
- `kind`: `provider_variant`, `family`, `alias`, or `selector`
- `provider_variant`: the exact execution target for the entry
- `family_id`: canonical cross-provider family identity
- `family_display_name`: human-facing family label
- `resolves_to`: exact provider variant chosen for aliases, selectors, and families

`context_window` is best-effort metadata only: Polaris does not hard-enforce provider token windows, and the upstream provider remains the source of truth for actual token-limit rejection. Aliases are NOT returned by this endpoint unless `include_aliases=true`, in which case alias, selector, and family entries are included with `resolves_to`.

#### Errors

Standard auth errors only.

#### curl

```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer $POLARIS_KEY"
```

---

## 13. Usage

### `GET /v1/usage`

Return usage statistics for the authenticated key over a time range.

**Auth:** required.

#### Query parameters

| Name | Type | Required | Notes |
|---|---|---|---|
| `from` | string (RFC 3339) | no | Start of the window (inclusive). Default: 30 days ago. |
| `to` | string (RFC 3339) | no | End of the window (exclusive). Default: now. |
| `model` | string | no | Filter to a specific `provider/model`. |
| `modality` | string | no | Filter to `chat`, `image`, `video`, `music`, `voice`, `audio`, or `embed`. |
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

`cost_usd` is an *estimate* computed from the cost tables in `internal/gateway/middleware/pricing.go`. It is NOT an authoritative bill. Unknown model costs default to `0` rather than blocking usage reporting.

For the Phase 5A runtime, `modality=music` is supported alongside `chat`, `image`, `video`, `voice`, `audio`, and `embed`.

#### Errors

Standard auth errors plus `400 invalid_request_error` if `from > to` or the date format is invalid.

#### curl

```bash
curl "http://localhost:8080/v1/usage?from=2026-04-01T00:00:00Z&group_by=model" \
  -H "Authorization: Bearer $POLARIS_KEY"
```

---

## 14. Control Plane & API Keys

The control-plane management surface is implemented, admin-only, and exposed only when `control_plane.enabled: true`. Use it in `auth.mode: virtual_keys` or `auth.mode: external`; `/v1/keys` remains available as a compatibility facade.

### `POST /v1/projects`, `GET /v1/projects`

Project is the primary container for virtual keys, policies, budgets, and usage attribution.

- `POST /v1/projects` request:
  - `name` required
  - `description` optional
- `GET /v1/projects` query:
  - `include_archived` optional boolean

Project response:

```json
{
  "id": "proj_key_...",
  "name": "acme-prod",
  "description": "Production tenant",
  "created_at": "2026-04-20T12:34:56Z",
  "archived_at": null
}
```

List responses use:

```json
{
  "object": "list",
  "data": [{ "...": "..." }]
}
```

### `POST /v1/virtual_keys`, `GET /v1/virtual_keys`, `DELETE /v1/virtual_keys/:id`

Issue, list, or revoke Polaris virtual keys.

| Field | Type | Required | Notes |
|---|---|---|---|
| `project_id` | string | yes | Owning project id. |
| `name` | string | yes | Human-readable label. |
| `rate_limit` | string | no | e.g. `100/min`. |
| `allowed_models` | array | no | Glob patterns. Default `["*"]`. |
| `allowed_modalities` | array | no | e.g. `["chat","image"]`. |
| `allowed_toolsets` | array | no | Toolset ids this key may use. |
| `allowed_mcp_bindings` | array | no | MCP binding ids this key may use. |
| `is_admin` | boolean | no | Grants control-plane admin access. |
| `expires_at` | string (RFC 3339) | no | Optional expiration. |

Virtual-key create response:

```json
{
  "id": "vk_key_...",
  "project_id": "proj_key_...",
  "name": "worker-eu-west",
  "key": "polaris-sk-live-abcdef1234567890...",
  "key_prefix": "polaris-",
  "rate_limit": "1000/min",
  "allowed_models": ["openai/*"],
  "allowed_modalities": ["chat", "image"],
  "allowed_toolsets": ["ts_key_..."],
  "allowed_mcp_bindings": ["mcp_key_..."],
  "is_admin": false,
  "created_at": "2026-04-20T12:34:56Z",
  "expires_at": null
}
```

**The full `key` is returned exactly once at creation time.** Later list calls omit it and return `key_prefix` only.

### `POST /v1/policies`, `GET /v1/policies`

Policies attach allowlists at the project level.

- `project_id` required
- `name` required
- `description` optional
- `allowed_models` optional, defaults to `["*"]`
- `allowed_modalities` optional
- `allowed_toolsets` optional
- `allowed_mcp_bindings` optional

### `POST /v1/budgets`, `GET /v1/budgets`

Budgets attach request or estimated-cost limits to a project.

- `project_id` required
- `name` required
- `mode` required: `soft` or `hard`
- `limit_usd` optional
- `limit_requests` optional
- `window` optional, defaults to `monthly`

Hard budgets can block requests with `429 budget_exceeded / budget_exceeded` once the current window has already exceeded the configured limit. Soft budgets are recorded but not enforced.

### `POST /v1/tools`, `GET /v1/tools`

Registers local tool definitions backed by operator-registered runtime implementations.

- `name` required
- `description` optional
- `implementation` required
- `input_schema` optional JSON Schema; if omitted Polaris uses the runtime schema when available
- `enabled` optional, defaults to `true`

Polaris does not upload arbitrary code. `implementation` must already exist in the runtime registry.

### `POST /v1/toolsets`, `GET /v1/toolsets`

Toolsets group tool definition ids into a reusable permission unit.

- `name` required
- `description` optional
- `tool_ids` required, at least one

### `POST /v1/mcp/bindings`, `GET /v1/mcp/bindings`

Registers MCP broker bindings.

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | Human-readable label. |
| `kind` | string | yes | `upstream_proxy` or `local_toolset`. |
| `upstream_url` | string | for `upstream_proxy` | Base URL of the upstream MCP server. |
| `toolset_id` | string | for `local_toolset` | Toolset exposed through the MCP broker. |
| `headers` | object | no | Static headers injected into upstream proxy requests. |
| `enabled` | boolean | no | Defaults to `true`. |

### Legacy compatibility: `POST /v1/keys`, `GET /v1/keys`, `DELETE /v1/keys/:id`

These endpoints remain implemented for compatibility.

- In `auth.mode: virtual_keys`, they read and write Polaris virtual keys using the older response shape (`owner_id` instead of `project_id`).
- In `auth.mode: multi-user`, they still operate on the legacy `api_keys` rows.

Legacy key request fields:

- `name` required
- `owner_id` optional compatibility alias for `project_id`
- `project_id` optional in `virtual_keys` mode
- `rate_limit`, `allowed_models`, `is_admin`, `expires_at` optional

#### Control-plane errors

| HTTP | `type` | Cause |
|---|---|---|
| 400 | `invalid_request_error` | Missing required fields, invalid `rate_limit`, bad budget mode, unknown tool/toolset, bad MCP binding kind. |
| 401 | `authentication_error` | Missing or invalid bootstrap admin / admin key. |
| 403 | `permission_error / admin_required` | Caller is not a control-plane admin. |
| 404 | `invalid_request_error / control_plane_disabled` | Control-plane management endpoints are disabled by config. |
| 404 | `invalid_request_error / key_not_found` or `binding_not_found` | Revoking a missing virtual key or using an unknown MCP binding. |

---

## 15. MCP Broker

### `GET /mcp/:binding_id`, `GET /mcp/:binding_id/*path`

Metadata probe for a configured MCP binding. For `local_toolset` bindings Polaris returns broker metadata:

```json
{
  "binding_id": "mcp_key_...",
  "kind": "local_toolset",
  "toolset_id": "ts_key_...",
  "transport": "streamable_http",
  "capabilities": { "tools": true }
}
```

For `upstream_proxy` bindings, `GET` is proxied upstream.

### `POST /mcp/:binding_id`, `POST /mcp/:binding_id/*path`

Current runtime supports streamable HTTP MCP only.

- `upstream_proxy`: Polaris forwards the incoming request to the configured `upstream_url`, preserving method, body, query string, and response body while adding any configured static headers.
- `local_toolset`: Polaris serves a small JSON-RPC-compatible MCP runtime with these methods:
  - `initialize`
  - `ping`
  - `tools/list`
  - `tools/call`

Example `tools/call` request:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "echo",
    "arguments": { "text": "hello" }
  }
}
```

Example successful `tools/call` response:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [{ "type": "text", "text": "hello" }],
    "structuredContent": { "text": "hello" },
    "isError": false
  }
}
```

The current built-in local implementations are:

- `echo`
- `time.now`
- `math.add`

Common MCP errors:

| HTTP | `type` | Cause |
|---|---|---|
| 400 | `invalid_request_error / missing_binding_id` | Missing binding id in the route. |
| 400 | `invalid_request_error / unsupported_mcp_method` | Local toolset binding received an unsupported method. |
| 403 | `permission_error / mcp_binding_not_allowed` | Key may not use the binding. |
| 403 | `permission_error / toolset_not_allowed` | Key may not use the local toolset behind the binding. |
| 403 | `permission_error / binding_disabled` | Binding exists but is disabled. |
| 404 | `invalid_request_error / binding_not_found` | Binding id does not exist. |
| 502 | `provider_error / mcp_proxy_failed` | Upstream MCP server could not be reached. |

---

## 16. Health & Readiness

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

## 17. Metrics

### `GET /metrics`

Implemented. Prometheus text-format metrics are exposed when `observability.metrics.enabled: true`. The route path is configured by `observability.metrics.path` at startup; the default is `/metrics`, and disabled runtimes return `404`.

When OTLP tracing is enabled, request traces now include child spans for auth lookup, rate-limit and budget evaluation, cache lookup/store, provider HTTP calls, fallback attempts, and tool/MCP execution. Polaris uses stable OpenTelemetry HTTP semantics plus low-cardinality `polaris.*` attributes such as `polaris.interface_family`, `polaris.token_source`, `polaris.cache_status`, `polaris.tool_name`, and `polaris.mcp_binding_id`.

Metric catalog (`BLUEPRINT.md` §13.2):

| Metric | Type | Labels |
|---|---|---|
| `polaris_requests_total` | counter | `interface_family`, `model`, `modality`, `status`, `provider` |
| `polaris_request_duration_seconds` | histogram | `interface_family`, `model`, `modality`, `provider` |
| `polaris_provider_latency_seconds` | histogram | `model`, `provider` |
| `polaris_tokens_total` | counter | `model`, `provider`, `direction`, `token_source` |
| `polaris_estimated_cost_usd` | counter | `model`, `provider` |
| `polaris_rate_limit_hits_total` | counter | `key_id` |
| `polaris_budget_denials_total` | counter | `project_id` |
| `polaris_provider_errors_total` | counter | `provider`, `error_type` |
| `polaris_failovers_total` | counter | `from_model`, `to_model` |
| `polaris_cache_events_total` | counter | `status`, `model` |
| `polaris_tool_invocations_total` | counter | `tool`, `status` |
| `polaris_mcp_requests_total` | counter | `binding`, `status` |
| `polaris_active_streams` | gauge | `model`, `provider` |

---

## 18. Full Endpoint Map

| Method | Path | Modality | Auth | Status | Section |
|---|---|---|---|---|---|
| `POST` | `/v1/chat/completions` | chat | required | implemented | [§6](#6-chat) |
| `POST` | `/v1/responses` | chat | required | implemented | [§6](#6-chat) |
| `POST` | `/v1/messages` | chat | required | implemented | [§6](#6-chat) |
| `POST` | `/v1/embeddings` | embed | required | implemented | [§7](#7-embeddings) |
| `POST` | `/v1/tokens/count` | — | required | implemented | [§6](#6-chat) |
| `POST` | `/v1/images/generations` | image | required | implemented | [§8](#8-images) |
| `POST` | `/v1/images/edits` | image | required | implemented | [§8](#8-images) |
| `POST` | `/v1/video/generations` | video | required | implemented | [§9](#9-video) |
| `GET` | `/v1/video/generations/:id` | video | required | implemented | [§9](#9-video) |
| `GET` | `/v1/video/generations/:id/content` | video | required | implemented | [§9](#9-video) |
| `DELETE` | `/v1/video/generations/:id` | video | required | implemented | [§9](#9-video) |
| `POST` | `/v1/music/generations` | music | required | implemented | [§10](#10-music) |
| `POST` | `/v1/music/edits` | music | required | implemented | [§10](#10-music) |
| `POST` | `/v1/music/stems` | music | required | implemented | [§10](#10-music) |
| `POST` | `/v1/music/lyrics` | music | required | implemented | [§10](#10-music) |
| `POST` | `/v1/music/plans` | music | required | implemented | [§10](#10-music) |
| `GET` | `/v1/music/jobs/:id` | music | required | implemented | [§10](#10-music) |
| `GET` | `/v1/music/jobs/:id/content` | music | required | implemented | [§10](#10-music) |
| `DELETE` | `/v1/music/jobs/:id` | music | required | implemented | [§10](#10-music) |
| `GET` | `/v1/voices` | voice catalog | required | implemented | [§11](#11-voice--tts--stt) |
| `GET` | `/v1/voices/:id` | voice catalog | required | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/voices/clones` | voice asset | required | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/voices/designs` | voice asset | required | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/voices/:id/retrain` | voice asset | required | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/voices/:id/activate` | voice asset | required | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/voices/:id/archive` | voice asset | required | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/voices/:id/unarchive` | voice asset | required | implemented | [§11](#11-voice--tts--stt) |
| `DELETE` | `/v1/voices/:id` | voice asset | required | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/audio/speech` | voice (TTS) | required | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/audio/transcriptions` | voice (STT) | required | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/audio/transcriptions/stream` | voice (streaming STT) | required | implemented | [§11](#11-voice--tts--stt) |
| `GET` | `/v1/audio/transcriptions/stream/:id/ws` | voice (streaming STT) | client secret | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/audio/notes` | notes | required | implemented | [§11](#11-voice--tts--stt) |
| `GET` | `/v1/audio/notes/:id` | notes | required | implemented | [§11](#11-voice--tts--stt) |
| `DELETE` | `/v1/audio/notes/:id` | notes | required | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/audio/podcasts` | podcast | required | implemented | [§11](#11-voice--tts--stt) |
| `GET` | `/v1/audio/podcasts/:id` | podcast | required | implemented | [§11](#11-voice--tts--stt) |
| `GET` | `/v1/audio/podcasts/:id/content` | podcast | required | implemented | [§11](#11-voice--tts--stt) |
| `DELETE` | `/v1/audio/podcasts/:id` | podcast | required | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/audio/interpreting/sessions` | interpreting | required | implemented | [§11](#11-voice--tts--stt) |
| `GET` | `/v1/audio/interpreting/sessions/:id/ws` | interpreting | client secret | implemented | [§11](#11-voice--tts--stt) |
| `POST` | `/v1/audio/sessions` | audio | required | implemented | [§11](#11-voice--tts--stt) |
| `GET` | `/v1/audio/sessions/:id/ws` | audio | client secret | implemented | [§11](#11-voice--tts--stt) |
| `GET` | `/v1/models` | — | required | implemented | [§12](#12-models) |
| `GET` | `/v1/usage` | — | required | implemented | [§13](#13-usage) |
| `POST` | `/v1/projects` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `GET` | `/v1/projects` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `POST` | `/v1/virtual_keys` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `GET` | `/v1/virtual_keys` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `DELETE` | `/v1/virtual_keys/:id` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `POST` | `/v1/policies` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `GET` | `/v1/policies` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `POST` | `/v1/budgets` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `GET` | `/v1/budgets` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `POST` | `/v1/tools` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `GET` | `/v1/tools` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `POST` | `/v1/toolsets` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `GET` | `/v1/toolsets` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `POST` | `/v1/mcp/bindings` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `GET` | `/v1/mcp/bindings` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `POST` | `/v1/keys` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `GET` | `/v1/keys` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `DELETE` | `/v1/keys/:id` | — | admin | implemented | [§14](#14-control-plane--api-keys) |
| `GET,POST` | `/mcp/:binding_id` | — | required | implemented | [§15](#15-mcp-broker) |
| `GET,POST` | `/mcp/:binding_id/*` | — | required | implemented | [§15](#15-mcp-broker) |
| `GET` | `/health` | — | none | implemented | [§16](#16-health--readiness) |
| `GET` | `/ready` | — | none | implemented | [§16](#16-health--readiness) |
| `GET` | `/metrics` | — | none | implemented | [§17](#17-metrics) |

---

*This document is versioned alongside the code. Any change to an endpoint's contract — path, body schema, response shape, error semantics, auth requirements — MUST land in the same commit as the code change.*
