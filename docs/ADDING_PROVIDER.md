# Adding a Provider

This guide is the implementation checklist for adding a provider to Polaris without weakening the gateway contract.

## Decision Rules

- Add providers only when there is a real adapter, config entry, catalog metadata, tests, and documentation.
- Keep provider code isolated in `internal/provider/<name>/`.
- Preserve provider-native model IDs internally; use aliases only at the Polaris routing layer.
- Prefer existing shared helpers in `internal/provider/common/` before adding new transport code.
- Do not add public endpoints for provider-specific features unless the shared Polaris contract needs that capability.

## Required Implementation Steps

1. Create `internal/provider/<name>/client.go` for base URL, auth headers, timeout, retry, and shared request helpers.
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
