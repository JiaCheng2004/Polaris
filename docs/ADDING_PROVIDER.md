# Adding a Provider

This guide is the implementation checklist for adding a provider to Polaris without weakening the gateway contract.

## Decision Rules

- Add providers only when there is a real adapter, config entry, catalog metadata, tests, and documentation.
- Keep provider code isolated in `internal/provider/<name>/`.
- Preserve provider-native model IDs internally; use aliases only at the Polaris routing layer.
- Build on the shared transport + adapter kit (below) before writing new transport code; most providers are a thin config over an existing compatibility base.
- Do not add public endpoints for provider-specific features unless the shared Polaris contract needs that capability.

## Required Implementation Steps

1. Create `internal/provider/<name>/client.go` — construct a `transport.Client` (see the kit below) with the provider's base URL, auth strategy, and error translator. Do not hand-roll HTTP, retries, or SSE.
2. Add one file per supported modality, such as `chat.go`, `embed.go`, `image.go`, `video.go`, `voice.go`, or `music.go`.
3. Implement only the relevant `internal/modality` interfaces.
4. Add `internal/provider/registry_<name>.go` with an `init()` call to `registerProviderFamilyRegistrar("<name>", register<Name>Provider)`.
5. Add the provider model metadata to `internal/provider/catalog/models.yaml`.
6. Add `config/providers/<name>.yaml` with environment-variable credential references and model `use` entries.
7. Add the provider import to the appropriate root config through `config/polaris.example.yaml` or a targeted smoke config.
8. Update `docs/PROVIDERS.md` with auth, supported modalities, endpoint notes, and known limitations.
9. Update `docs/API_REFERENCE.md` and `spec/openapi/polaris.v1.yaml` only if a public Polaris endpoint or wire contract changes.
10. Add unit tests with `httptest.NewServer`; provider tests must not call real upstream APIs.
11. Add live-smoke cases only when credentials, quotas, and plan access can prove the path.

## Registration Pattern

Provider registration is compile-time modular. Each provider owns its registrar:

```go
func init() {
    registerProviderFamilyRegistrar("example", registerExampleProvider)
}
```

The central registry validates duplicates and exposes a sorted supported-provider list. Do not edit the registry factory map directly; there is no central map to extend.

## The transport + adapter kit

Provider code is thin because the hard parts are shared. Build on these, in order of preference:

- **`internal/transport`** — the single outbound HTTP core. One `transport.Client` gives you jittered exponential backoff, `Retry-After` honoring, per-attempt timeouts, a reusable SSE decoder, an outbound SSRF-safe client, and pluggable auth strategies (bearer, header map, `x-api-key`+version, SigV4, OAuth token source). No provider hand-rolls HTTP, retries, or streaming.
- **`internal/provider/openaicompat` and `internal/provider/anthropiccompat`** — the two compatibility bases, both reparented onto `transport`. If the upstream speaks the OpenAI or Anthropic wire format (most do), your provider is a thin config over one of these: a base URL, an auth decorator, an error translator, and optional per-request tweaks (extra headers, a request translator, thinking / hosted-tool options).
- **`internal/provider/core`** — shared adapter helpers (model-name normalization, content and tool-choice translation, safe numeric conversions) reused across adapters and the verification package.

**The thin-provider pattern.** A provider whose wire format already matches a base is a few lines: its `registry_<name>.go` constructs the client and registers models against the shared base. For example, `deepseek`:

```go
func registerDeepSeekProvider(registry *Registry, warnings *[]string, name string, cfg config.ProviderConfig) {
    client := deepseek.NewClient(cfg) // wraps openaicompat, which is built on transport
    registerChatOnlyModels(registry, warnings, name, cfg.Models, func(modelID string) modality.ChatAdapter {
        return deepseek.NewChatAdapter(client, modelID)
    })
}
```

Reach for a hand-written adapter only when the upstream wire format is genuinely different (Google Gemini, Volcengine Ark, ElevenLabs). Even then the client builds on `transport` and reuses `core` helpers, and provider packages never import `internal/gateway` (enforced by `make check-layering`).

## Test Requirements

- Adapter unit tests must prove request translation, auth headers, provider errors, and response normalization.
- Streaming adapters must test chunk parsing and provider error events.
- Async adapters must test submit, poll, content, cancellation, and missing-job behavior when supported.
- Config tests must prove the provider config loads through `make config-check`.
- Contract tests must pass when public routes or errors are touched.

## Validation Commands

```bash
make fmt-check
make lint
make config-check
make contract-check
make security-check
make test
```

Use `make live-smoke` only when provider credentials, quota, and plan access are available.
