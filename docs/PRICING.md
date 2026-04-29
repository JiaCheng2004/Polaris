# Pricing

Polaris estimates request cost through `internal/pricing`, a YAML-driven catalog loaded at startup and read through an atomic holder. Bundled defaults live in `internal/pricing/data/*.yaml`; operators can add or replace entries with `runtime.pricing.file`.

Cost estimates are operational telemetry, not provider invoices. Missing models return `$0` with `cost_source=missing`; requests with no billable dimensions return `$0` with `cost_source=fallback_zero`.

## Runtime Config

```yaml
runtime:
  pricing:
    file: ./pricing.yaml
    reload_interval_seconds: 30
    fail_on_missing: false
```

- `file`: optional override file. Bundled defaults always load first.
- `reload_interval_seconds`: polling interval for override hot reload. `0` disables the pricing watcher.
- `fail_on_missing`: when true, resolved models without a pricing entry return `400 model_not_priced` before the provider call. Default behavior remains graceful degradation to `$0`.

Overrides replace bundled entries by exact `provider/model` key. New keys are added. Wildcards are supported as suffix matches, for example `ollama/*`.

## Schema

```yaml
version: 1
models:
  openai/gpt-4o:
    mode: chat
    currency: USD
    context_window: 128000
    source: https://platform.openai.com/docs/pricing
    effective_from: 2026-04-01
    pricing:
      input_per_mtok: 5.00
      output_per_mtok: 15.00
      cache_read_per_mtok: 2.50
    tiers:
      batch: { multiplier: 0.5 }
      flex: { input_per_mtok: 2.50, output_per_mtok: 7.50 }
```

Supported token rates include `input_per_mtok`, `output_per_mtok`, `output_reasoning_per_mtok`, cache read/write rates, DeepSeek-style `input_cache_hit_per_mtok`, and image token rates.

Supported non-token rates include `input_per_audio_second`, `input_per_video_second`, `input_per_character`, `input_per_image`, `output_per_image`, `output_per_pixel`, and `per_call`.

Use `tiered_pricing` for context-length bands:

```yaml
tiered_pricing:
  - id: lte_32k
    range: [0, 32000]
    input_per_mtok: 1.20
    output_per_mtok: 6.00
```

Use `additional_units` for tool surcharges:

```yaml
additional_units:
  web_search_per_1k_calls: 10.00
```

The estimator accepts unit counts such as `web_search: 3` and applies the matching `_per_call`, `_per_1k_calls`, or `_per_container_hour` rate.

## Adding A Model

Add the entry to the provider YAML in `internal/pricing/data/`, include `source` and `effective_from`, then run:

```bash
go test ./internal/pricing
go test ./internal/gateway/middleware
```

If the API surface or usage response changes, update `docs/API_REFERENCE.md` and `spec/openapi/polaris.v1.yaml` in the same change.
