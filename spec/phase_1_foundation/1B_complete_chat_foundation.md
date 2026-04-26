# Phase 1B: Complete the Chat Foundation

**Status:** Planned

## Summary

This milestone completes the missing feature work inside Phase 1. The goal is to move Polaris from a bootable kernel to a working chat gateway that can serve real OpenAI-compatible chat traffic through OpenAI and Anthropic, then record and report usage.

This is still Phase 1. It must not pull in Redis, PostgreSQL, hot reload, failover execution, metrics, or additional providers.

## Key Changes

- **Registry and provider wiring**
  - Upgrade the registry from metadata-only to adapter-aware.
  - Register real chat adapters for configured OpenAI and Anthropic models.
  - Expose `GetChatAdapter(modelOrAlias)` alongside the existing model metadata lookup.
  - Keep all non-OpenAI and non-Anthropic provider packages as placeholders in this milestone.

- **Provider clients**
  - Implement OpenAI and Anthropic `client.go` files with:
    - auth headers
    - base URL handling
    - timeout handling
    - retry policy execution from config
    - upstream error classification into Polaris error types
  - Use `net/http` only. No provider SDKs.

- **Provider chat adapters**
  - OpenAI adapter:
    - call the OpenAI chat-completions endpoint
    - support non-streaming and SSE streaming
    - normalize provider responses into `modality.ChatResponse` and `modality.ChatChunk`
  - Anthropic adapter:
    - translate system messages into Anthropic's top-level `system` field
    - translate message and tool structures into Anthropic format
    - synthesize a default `max_tokens` when omitted
    - normalize Anthropic streaming events into OpenAI-compatible chunks

- **Chat endpoint**
  - Implement `POST /v1/chat/completions`.
  - Validate request shape before provider calls:
    - non-empty messages
    - valid roles
    - valid numeric ranges
    - no more than 4 stop sequences
    - valid tool and response-format shape
  - Resolve aliases and canonical model names through the registry.
  - Enforce modality and capability checks before the adapter call.
  - Support:
    - standard JSON response when `stream=false`
    - SSE when `stream=true`
  - SSE rules:
    - correct response headers
    - flush after every `data:` frame
    - emit final chunk with `usage`
    - terminate with `data: [DONE]`
    - if a mid-stream error occurs, emit a single error frame and then `[DONE]`

- **Capability model**
  - Add `json_mode` as a capability and use it to gate `response_format`.
  - Continue to gate request features by model capability:
    - `vision`
    - `audio_input`
    - `function_calling`
    - `streaming`
    - `json_mode`

- **Usage tracking**
  - Implement a typed request-outcome payload in Gin context.
  - Handlers and adapters populate the outcome with:
    - request ID
    - resolved model
    - modality
    - provider latency
    - total latency
    - token usage
    - error type
    - status code
  - `middleware/usage.go` converts that payload into `store.RequestLog` and writes via the async logger.
  - Do not parse response bodies in middleware.

- **Usage endpoint**
  - Implement `GET /v1/usage` for the authenticated key.
  - Support the query contract already defined in `docs/API_REFERENCE.md`:
    - `from`
    - `to`
    - `model`
    - `modality`
    - `group_by=day|model`
  - Use the existing store aggregation methods.
  - Phase 1 pricing only needs the shipped OpenAI and Anthropic chat models.
  - Unknown models must return `cost_usd: 0` instead of failing the request.

- **Docs and config alignment**
  - Update `docs/API_REFERENCE.md` in the same change as the endpoint implementation.
  - Update reference config capability lists so OpenAI and Anthropic models that support structured output include `json_mode`.
  - Do not claim failover or metrics are implemented in this milestone.

## Public Interfaces Added Or Changed

- new `POST /v1/chat/completions`
- new `GET /v1/usage`
- registry gains chat-adapter lookup by canonical model or alias
- capability enum gains `json_mode`
- internal request-outcome context payload becomes the contract between handler, middleware, and async usage logging

## Test Plan

- **OpenAI adapter tests**
  - happy path
  - streaming path
  - upstream 401
  - upstream 429
  - upstream 5xx
  - timeout

- **Anthropic adapter tests**
  - system-message translation
  - non-streaming path
  - streaming event normalization
  - missing `max_tokens` defaulting
  - upstream error mapping

- **HTTP tests**
  - valid non-streaming chat request
  - valid streaming chat request
  - alias resolution
  - unknown model
  - modality mismatch
  - capability mismatch
  - malformed request body
  - static-auth denial
  - mid-stream error behavior

- **Usage tests**
  - request-outcome to `RequestLog` conversion
  - async logging writes expected rows
  - `GET /v1/usage` with day grouping
  - `GET /v1/usage` with model grouping
  - invalid date and invalid `group_by` handling
  - authenticated-key scoping

## Exit Criteria

`1B` is complete only when:

- OpenAI and Anthropic chat calls work end to end
- JSON and SSE chat responses both work
- usage is recorded asynchronously in SQLite
- `GET /v1/usage` reports the authenticated key's usage
- the API reference matches the implemented chat and usage surfaces
- all new tests pass under `go test -race ./...`
