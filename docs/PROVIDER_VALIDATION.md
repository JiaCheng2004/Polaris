# Provider Validation

This records the live validation of the curated 19-provider set. Reproduce the
live checks with sandbox keys in the environment:

```bash
set -a; . ./.env; set +a
go test -tags steelthread ./tests/e2e -run TestSteelThreadChat -v
```

The steel-thread builds the real provider registry and adapter for each provider
and makes one live chat call against a current model, proving base URL, auth,
request translation, and response parsing work end to end (not mocked).

## Results (validated 2026-07-06 with sandbox keys)

### Live end-to-end through Polaris — PASS

Each returned a valid completion via `adapter.Complete` through the real API:

| Provider | Model | Result |
|---|---|---|
| openai | `gpt-4o` | ✅ reply "OK" |
| anthropic | `claude-opus-4-8` | ✅ reply "OK" |
| deepseek | `deepseek-v4-flash` | ✅ reply "OK" |
| xai | `grok-4.20-0309-non-reasoning` | ✅ reply "OK" |
| google | `gemini-2.5-flash` | ✅ reply "OK" |
| bytedance (Volcengine Ark) | `doubao-1-5-pro-32k-250115` | ✅ reply "OK" |

This covers every major adapter family: `openaicompat` (openai, deepseek, xai —
and by extension groq/together/openrouter/fireworks/nvidia/mistral, which share
it), `anthropiccompat` (anthropic), the native Google Gemini adapter, and the
Volcengine Ark adapter.

### Key + endpoint validated (non-chat surfaces)

| Provider | Surface | Check |
|---|---|---|
| elevenlabs | voice/music | `GET /v1/models` → 200 (key + base URL correct) |
| minimax | music/audio/video | `POST /v1/music_generation` → 400 (auth accepted; endpoint + base URL correct) |

minimax is a music/audio/video provider in Polaris; its chat surface was the
pruned token-plan variant.

### Not live-tested

| Provider | Reason |
|---|---|
| qwen (DashScope) | No usable sandbox key present (`DASHSCOPE_API_KEY` empty). Base URL `https://dashscope.aliyuncs.com/compatible-mode/v1` and OpenAI-compatible auth are correct by construction; the adapter is the shared `openaicompat` client. |
| bedrock, mistral, groq, together, openrouter, fireworks, nvidia, ollama, replicate, google-vertex | No sandbox key. Validated statically: correct base URLs + auth schemes, and each is either the `openaicompat` family (identical to the live-proven openai/deepseek/xai path) or covered by `httptest`-based unit tests. |

## Note on model catalogs

The embedded catalog (`internal/provider/catalog/models.yaml`) is curated
metadata (aliases, capabilities, pricing) and tracks provider model releases;
operators configure the specific models they have access to. Provider model
lineups move quickly — the models above are the ones verified live on the
validation date. The **integration** (base URL, auth, wire translation) is what
the steel-thread proves, and it is stable across a provider's model releases.
