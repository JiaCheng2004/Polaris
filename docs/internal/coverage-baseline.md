# Coverage Baseline (pre-overhaul)

Captured at the start of the v1.0.0 overhaul on branch `overhaul/v1`, Go 1.26.4, `go test -count=1 -coverprofile ./internal/... ./pkg/...`.

> **Caveat:** these are per-package profile numbers. Go does not credit cross-package execution without `-coverpkg`, so integration-exercised packages (notably `internal/gateway/handler`, driven end-to-end by `internal/gateway/server_test.go`) read far lower than their true exercised percentage. Treat low handler/middleware numbers as "weak *direct* unit coverage," not "unexercised." These figures exist only to set the ratchet floors in the plan (§7.2); the campaign target is ≥75% aggregate with per-package floors.

Aggregate profile coverage: **32.7%**.

## Packages with test files (worst first)

| Coverage | Package |
|---|---|
| 3.6% | internal/gateway/handler |
| 15.8% | internal/store/blob |
| 17.4% | internal/gateway/middleware |
| 22.2% | internal/provider/nvidia |
| 22.5% | internal/modality |
| 34.6% | internal/store/sqlite |
| 37.0% | internal/provider/anthropic |
| 43.1% | internal/provider/google |
| 43.2% | internal/provider/qwen |
| 46.5% | internal/provider/common/anthropiccompat |
| 47.2% | internal/provider/common/aws |
| 48.4% | internal/provider/bytedance |
| 49.3% | internal/provider/minimax |
| 52.2% | internal/provider/openai |
| 53.7% | pkg/client |
| 53.9% | internal/provider/common/openaicompat |
| 54.8% | internal/provider/bedrock |
| 55.9% | internal/provider/ollama |
| 57.5% | internal/provider/elevenlabs |
| 57.6% | internal/provider/replicate |
| 58.7% | internal/gateway/httputil |
| 60.0% | internal/provider |
| 60.4% | internal/provider/googlevertex |
| 62.0% | internal/gateway/runtime |
| 62.1% | internal/pricing |
| 64.7% | internal/provider/common/auth |
| 65.0% | internal/config |
| 65.1% | internal/provider/catalog |
| 66.7% | internal/provider/zaitoken |
| 67.5% | internal/understanding |
| 68.6% | internal/provider/verification |
| 85.7% | internal/provider/minimaxtoken |
| 96.0% | internal/gateway |
| 100% | internal/provider/deepseek, internal/provider/xai |

## Packages with no test files (0.0%)

`internal/store` (async request/audit loggers), `internal/store/cache` (rate-limit backing store), `internal/store/postgres` (only reached by env-gated integration tests), `internal/provider/common/retry`, `internal/gateway/metrics`, `internal/gateway/telemetry`, `internal/tooling`, `internal/provider/audiohelper`, `internal/provider/common/chattools`, `internal/provider/common/contracttest`, `internal/provider/common/files`, `internal/provider/common/safeconv`, the 4 `internal/provider/bytedance/astpb/*` protobuf packages, and 8 thin OpenAI-compatible providers (featherless, fireworks, glm, groq, mistral, moonshot, openrouter, together).

## Ratchet targets (plan §7.2)

transport ≥90 · reliability ≥90 · routing ≥90 · guardrails ≥90 · semcache ≥90 · mcp ≥85 · store ≥85 · store/cache ≥85 · middleware ≥85 · provider/core ≥90 · handler ≥70 direct · aggregate ratcheted to ≥75%.
