# Phase 2E: Runtime Routing and Failover

**Status:** Implemented

## Summary

This milestone finishes the runtime routing behavior that becomes meaningful once the Phase 2 provider matrix is broad enough. Alias resolution already exists in the registry. This step turns aliases and fallbacks into full runtime behavior across the expanded chat path.

## Key Changes

- **Model resolution**
  - Preserve the documented routing order:
    1. resolve alias if needed
    2. resolve canonical `provider/model`
    3. check modality
    4. check key permissions against the resolved canonical model
    5. return adapter or error
  - Keep alias handling config-driven only.
  - Do not introduce code-defined aliases.

- **Failover execution**
  - Implement failover in the chat handler path using `routing.fallbacks`.
  - Only fail over on retryable upstream failures:
    - `429`
    - provider timeout
    - provider `5xx`
  - Never fail over on:
    - `400`
    - `401`
    - `403`
    - `422`
  - Try fallback targets strictly in the configured order.
  - Stop at the first successful fallback response.
  - If a fallback serves the request, set `X-Polaris-Fallback: <provider/model>`.

- **Usage and logging**
  - Ensure request-outcome logging records:
    - requested model or alias only if explicitly retained for logging
    - actual serving canonical model
    - actual serving provider
  - Ensure metrics and logs reflect the final serving provider, not only the requested alias.

- **Error behavior**
  - If all fallbacks fail, return the final retryable provider error.
  - Keep failover behavior invisible for non-retryable error classes.
  - Preserve OpenAI-compatible error envelopes even when multiple upstream attempts occur.

## Public Interfaces Added Or Changed

- no new HTTP endpoints
- `/v1/chat/completions` gains:
  - config-driven runtime failover
  - `X-Polaris-Fallback` response header on successful fallback

## Test Plan

- handler tests:
  - retryable upstream failure followed by successful fallback
  - multiple fallback targets with first-success stop behavior
  - fallback exhaustion
  - non-retryable upstream error with no fallback attempt
- authorization tests:
  - alias resolves before permission check
  - permission check applies to the resolved canonical model
- usage/logging tests:
  - final serving model/provider are recorded correctly after fallback

## Exit Criteria

`2E` is complete only when:

- retryable errors trigger configured fallback attempts
- non-retryable errors never trigger fallback
- successful fallback responses emit `X-Polaris-Fallback`
- logs and usage records reflect the actual serving provider/model
- failover tests pass under `go test -race ./...`
