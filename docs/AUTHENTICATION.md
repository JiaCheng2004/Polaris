# Polaris Authentication

Polaris intentionally separates gateway authorization from application login. If another product already owns users, sessions, Google OAuth, phone OTP, SSO, or organization membership, keep that logic in the product backend and use `auth.mode: external`.

## Choosing A Mode

Use the simplest mode that matches the deployment:

- `none`: local development only. No credentials required.
- `static`: one or more fixed bearer keys from YAML.
- `external`: bring-your-own-auth. A trusted upstream app signs claims for Polaris.
- `virtual_keys`: Polaris-owned API keys, projects, policies, budgets, toolsets, MCP bindings, and audit.
- `multi-user`: legacy database key compatibility.

For embedding Polaris into another platform quickly, use `external`. For exposing Polaris directly as a gateway product, use `virtual_keys`.

## Mode Comparison

| Mode | Identity lives in | Best for |
|---|---|---|
| `none` | — | Local development only. |
| `static` | YAML (`sha256:` hashes) | Small trusted deployments, CI. |
| `external` | Your app (signed claims) | Embedding Polaris behind an app that already owns users. |
| `virtual_keys` | Polaris store (projects, policies, budgets, audit) | Exposing Polaris directly as a gateway product. |
| `multi-user` | Database keys | Legacy compatibility. |

Every mode enforces the same downstream checks — model, modality, toolset, MCP-binding, rate-limit, and budget — so switching modes changes only *where* identity comes from, not *what* it is allowed to do.

## Keys And Hashing

Polaris never stores or logs plaintext API keys. It persists only the SHA-256 hash; the 8-character `key_prefix` is the only key material that appears in logs.

For `static` keys and the `virtual_keys` bootstrap admin key, **you choose the key** — Polaris only stores its hash. Produce the hash with nothing but `openssl` (no Go toolchain needed, so it works from the published container too):

```bash
# The key you choose is the bearer token you will actually use. Keep it.
export POLARIS_ADMIN_KEY=$(openssl rand -hex 32)
# The value to store in config (only the hash is persisted):
echo "sha256:$(printf '%s' "$POLARIS_ADMIN_KEY" | openssl dgst -sha256 -r | awk '{print $1}')"
```

Put the printed `sha256:…` into `auth.static[].key_hash` or `auth.bootstrap_admin_key_hash` (or the `POLARIS_BOOTSTRAP_ADMIN_KEY_HASH` env), and authenticate with the raw `$POLARIS_ADMIN_KEY`. Commit only the hash, never the raw key. If you have a Go toolchain, `go run ./scripts/generate-key.go` is an equivalent alternative. See [CONFIGURATION.md](CONFIGURATION.md) for the exact `auth.static` and `auth.virtual_keys` shapes.

## External Signed Headers

Configure:

```yaml
auth:
  mode: external
  external:
    provider: signed_headers
    shared_secret: ${POLARIS_EXTERNAL_AUTH_SECRET}
    max_clock_skew: 60s
    cache_ttl: 60s
```

The upstream app validates its own user/session and forwards three headers:

- `X-Polaris-External-Auth`: base64url-encoded JSON claims
- `X-Polaris-External-Auth-Timestamp`: Unix seconds
- `X-Polaris-External-Auth-Signature`: `v1=<hex hmac-sha256>` over `timestamp + "\n" + encoded_claims`

Minimal claims:

```json
{
  "sub": "user_123",
  "allowed_models": ["openai/*"],
  "allowed_modalities": ["chat"]
}
```

Admin/control-plane claims:

```json
{
  "sub": "admin_123",
  "project_id": "proj_123",
  "is_admin": true,
  "allowed_models": ["*"],
  "allowed_modalities": ["chat", "embed", "image", "voice", "video", "audio", "music"]
}
```

Supported claim fields are documented in [API_REFERENCE.md](./API_REFERENCE.md#external-auth).

## Integration Pattern

1. The client authenticates with your app using Google OAuth, SMS OTP, SSO, password login, or any custom method.
2. Your app validates the session and maps its user/org/role data into Polaris claims.
3. Your app signs the claims with `POLARIS_EXTERNAL_AUTH_SECRET`.
4. Your app forwards the request to Polaris with the signed headers.
5. Polaris verifies the signature, timestamp, optional `expires_at`, and then applies the same model, modality, toolset, MCP, rate-limit, budget, and admin checks used by native Polaris auth.

Do not put provider API keys in client apps. Provider credentials stay in Polaris config. Product authentication stays in your app.
