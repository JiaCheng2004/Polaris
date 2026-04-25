# Phase 3H: Usage, Models, and Costing Reconciliation

**Status:** Implemented

## Summary

This milestone aligned the cross-cutting runtime surfaces after embeddings, images, and voice became real. The shipped implementation makes model metadata, request logging, cost estimation, and usage reporting describe the same system across every implemented Phase 3 modality.

## Key Changes

- **Model catalog**
  - Extend `GET /v1/models` so non-chat models return the metadata already declared in config:
    - embeddings: `dimensions`
    - images: `output_formats`
    - voice TTS: `voices`
    - voice STT: `formats`
  - Preserve the existing chat model behavior.

- **Usage logging**
  - Extend the async usage logging path to handle all Phase 3 modalities.
  - Keep the existing `request_logs` schema and `/v1/usage` response shape unchanged.
  - Populate:
    - embeddings: real token counts when available
    - images: zero tokens unless the provider returns counts
    - voice: zero tokens unless the provider returns counts
  - Always record request count, latency, modality, model, provider, status code, and estimated cost.

- **Cost tables**
  - Add Phase 3 pricing tables for the implemented image, voice, and embedding models where cost data is available in project policy.
  - The current shipped tables include the approved OpenAI embedding entries and preserve zero-cost defaults for models without project pricing.
  - Default unknown or intentionally unset costs to `0` rather than failing the request.
  - Keep cost reporting clearly labeled as an estimate.

- **Permission and rate-limit consistency**
  - Ensure `allowed_models` and rate-limit enforcement behave identically across chat, embeddings, images, and voice.
  - Ensure aliases resolve to canonical models before permission checks for every Phase 3 modality.

## Public Interfaces Added Or Changed

- No new endpoint paths
- Expanded behavior for:
  - `GET /v1/models`
  - `GET /v1/usage`
- `GET /v1/usage` continues to expose:
  - `total_requests`
  - `total_tokens`
  - `total_cost_usd`
  - `by_day` or `by_model`

## Test Plan

- `/v1/models` tests for embed, image, and voice metadata
- `/v1/usage` tests covering:
  - embedding requests with tokens
  - image requests with zero-token aggregation
  - voice requests with zero-token aggregation
  - grouping by day and model across mixed modalities
- Permission and rate-limit tests for non-chat modalities
- Cost-estimation tests for known-cost and unknown-cost models

## Exit Criteria

`3H` is complete only when:

- `/v1/models` is truthful for every implemented Phase 3 model type
- `/v1/usage` aggregates the new modalities without schema changes or regressions
- permission, alias, and rate-limit behavior is modality-consistent
- cost reporting remains stable and non-blocking even when a model lacks a pricing entry

## Non-Goals

`3H` must not add:

- new providers
- SDK implementation
- video or audio work

Those belong elsewhere in Phase 3 or Phase 4.
