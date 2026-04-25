# Phase 2G: Observability and Dashboards

**Status:** Implemented

## Summary

This milestone implements the Blueprint observability layer after the expanded provider matrix, distributed runtime, and failover behavior are real. Instrumentation should reflect the actual serving provider and actual runtime path, not just the requested model string.

## Key Changes

- **Prometheus metrics**
  - Implement `GET /metrics`.
  - Expose the Blueprint metric catalog:
    - `polaris_requests_total`
    - `polaris_request_duration_seconds`
    - `polaris_provider_latency_seconds`
    - `polaris_tokens_total`
    - `polaris_estimated_cost_usd`
    - `polaris_rate_limit_hits_total`
    - `polaris_provider_errors_total`
    - `polaris_failovers_total`
    - `polaris_active_streams`
  - Guard the route with `observability.metrics.enabled`.
  - Use `observability.metrics.path` as the route path.

- **Instrumentation points**
  - request start / end
  - provider latency
  - token accounting
  - estimated cost accounting
  - rate-limit rejections
  - provider error classes
  - fallback execution
  - active stream count
  - Ensure labels reflect:
    - actual serving provider
    - actual serving model
    - modality
    - status or error type as required by the metric

- **Structured logging alignment**
  - Keep log fields aligned with the Blueprint logging requirements:
    - `request_id`
    - `model`
    - `provider`
    - `modality`
    - `latency_ms`
    - `status`
    - `tokens`
    - `key_id` or key prefix
  - Ensure fallback-served requests are visible in logs.

- **Grafana assets**
  - Update `deployments/grafana/dashboards/polaris.json` so it matches the actual metric names and labels emitted by the server.
  - Make `STACK=dev` the default observability validation path.

## Public Interfaces Added Or Changed

- new `GET /metrics`
- metrics path becomes config-driven through `observability.metrics.path`

## Test Plan

- route tests:
  - metrics enabled
  - metrics disabled
  - custom metrics path
- metric content tests:
  - request counter increments
  - provider error counter increments
  - rate-limit counter increments
  - failover counter increments
  - active-stream gauge increments and decrements correctly
- dashboard validation:
  - dashboard JSON references the emitted metric names and labels

## Exit Criteria

`2G` is complete only when:

- `/metrics` emits the Blueprint metric catalog
- instrumentation reflects the actual runtime behavior including fallback
- the Grafana dashboard matches the exported metrics
- `STACK=dev` is a real observability validation path
