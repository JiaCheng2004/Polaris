# Benchmarks

Polaris publishes the **gateway-added overhead** and the per-feature costs of its
frontier subsystems. These are honest microbenchmarks, not a marketing throughput
claim: in real traffic, upstream provider latency dominates end-to-end time by
two to three orders of magnitude. Polaris competes on completeness and
reliability; these numbers show the gateway layer stays cheap.

## Methodology

- **In-process**: the full Gin middleware + handler stack is driven against
  deterministic in-memory / loopback mock providers, so the numbers isolate the
  gateway's own cost with zero network or provider I/O.
- **Reproduce**: `make bench` (or the `go test -bench` commands below). Numbers
  below were captured on Apple Silicon (10 threads), Go 1.26.4. Absolute values
  vary by hardware; the committed CI baseline runs on a pinned Linux runner and
  is compared with `benchstat` to fail regressions.

## Gateway overhead

Full middleware + handler stack, measured two ways:

| Path | ns/op | µs/op | B/op | allocs/op |
|---|---|---|---|---|
| Error path (missing model — pure gateway, zero provider I/O) | 13,456 | ~13.5 | 13,961 | 115 |
| Chat completion (full happy path vs a loopback mock provider) | 105,650 | ~106 | 34,139 | 302 |

The error path is the truest measure of gateway-added latency: **~13.5 µs** to
route, authorize, and shape a request before any provider is touched.

```bash
go test ./internal/gateway -run xxx -bench BenchmarkGatewayOverhead -benchmem
```

## Feature overhead (frontier subsystems)

Each frontier system has a benchmark-gated budget:

| Subsystem | Benchmark | Result | Budget | Status |
|---|---|---|---|---|
| Adaptive routing | `BenchmarkRouterOrderAdaptive` | **0.32 µs/op** (324 ns, 5 allocs) | < 10 µs p99 | ✅ |
| Guardrails — PII (4 KB) | `BenchmarkPII4KB` | **0.45 ms** | < 1 ms | ✅ |
| Guardrails — secrets (4 KB) | `BenchmarkSecrets4KB` | **0.25 ms** | < 1 ms | ✅ |
| Guardrails — prompt injection (4 KB) | `BenchmarkInject4KB` | **0.90 ms** | < 1 ms | ✅ |
| Guardrails — all detectors (4 KB) | `BenchmarkEngineEvaluate4KB` | **1.63 ms** | (combined) | — |
| Semantic cache lookup | `BenchmarkSemcacheLookup5K` | **2.7 ms** @ 5K × 1536-dim | < 5 ms p99 | ✅ |

Routing costs a third of a microsecond. Each local guardrail detector clears the
sub-millisecond bar on a 4 KB payload. A semantic-cache lookup over 5,000
1536-dimension vectors — a flat, contiguous cosine scan with no external index —
completes in under 3 ms.

## Interpreting these numbers

- **Guardrails and the semantic cache are disabled by default.** They only add
  cost on routes an operator opts in.
- **The gateway overhead (~13.5 µs) is dwarfed by provider latency** (typically
  hundreds of milliseconds to seconds). Polaris's value is what it does with that
  request — routing, failover, guardrails, caching, governance — not shaving
  microseconds off an already provider-bound call.
- We do not publish a "faster than X" claim. Where raw proxy throughput is the
  goal, other gateways are purpose-built for it; Polaris optimizes for a
  complete, reliable, multi-modal gateway in one self-hostable Go binary.
