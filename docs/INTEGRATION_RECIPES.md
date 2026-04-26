# Integration Recipes

These recipes cover the fastest ways to embed Polaris in another project.

## Local Ollama

Use this when you want a zero-cloud local development gateway.

```yaml
version: 2
imports:
  - ./providers/ollama.yaml
  - ./routing/local.yaml
runtime:
  server:
    host: 127.0.0.1
    port: 8080
  auth:
    mode: none
```

Run:

```bash
ollama serve
make run CONFIG=./config/polaris.yaml
curl http://127.0.0.1:8080/v1/models
```

## OpenAI-Compatible Provider

Use this for providers such as OpenRouter, Together, Groq, Fireworks, Featherless, Moonshot, GLM, or Mistral.

```yaml
version: 2
imports:
  - ./providers/openrouter.yaml
runtime:
  auth:
    mode: static
    static_keys:
      - name: local-app
        key_hash: sha256:<hash>
        allowed_models: ["*"]
```

Set the provider key through the environment:

```bash
export OPENROUTER_API_KEY=...
go run ./scripts/generate-key.go
make run CONFIG=./config/polaris.example.yaml
```

Call Polaris with the local gateway key, not the provider key:

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer <raw-local-key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"openrouter-chat","messages":[{"role":"user","content":"hello"}]}'
```

## External Signed Auth

Use this when your application owns login, sessions, Google OAuth, SSO, SMS OTP, or user lifecycle.

```yaml
runtime:
  auth:
    mode: external
    external:
      provider: signed_headers
      shared_secret: ${POLARIS_EXTERNAL_AUTH_SECRET}
      max_clock_skew: 60s
      cache_ttl: 60s
```

Your application signs the user/project/model claims and forwards the request to Polaris. Polaris verifies the signature and enforces the declared model permissions without owning your user database.

## Virtual Keys

Use this when Polaris should own projects, gateway keys, policies, budgets, toolsets, and audit records.

```yaml
runtime:
  auth:
    mode: virtual_keys
    bootstrap_admin_key_hash: sha256:<hash>
  control_plane:
    enabled: true
```

Create a project and virtual key:

```bash
curl http://127.0.0.1:8080/v1/projects \
  -H "Authorization: Bearer <bootstrap-admin-key>" \
  -H "Content-Type: application/json" \
  -d '{"name":"Example App"}'
```

Store only hashes in config or storage. Never commit raw gateway keys.

## Live Smoke

Use live smoke to prove real provider access after repo-local checks pass.

```bash
POLARIS_LIVE_SMOKE=1 make live-smoke
POLARIS_LIVE_SMOKE=1 POLARIS_LIVE_SMOKE_STRICT=1 make live-smoke
```

Credentials, quota, billing, and provider plan access are manual blockers for claiming live-provider proof. They are not required for local open-source development.
