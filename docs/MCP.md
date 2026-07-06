# MCP Gateway

Polaris speaks the **Model Context Protocol** as a stateless, streamable-HTTP
server, and can proxy or aggregate upstream MCP servers. It turns MCP tool traffic
into first-class governed traffic — every call flows through the same auth,
scoping, budget, and metrics as the rest of the gateway.

## Transport

- A single `POST` endpoint accepts JSON-RPC 2.0 (`initialize`, `ping`,
  `tools/list`, `tools/call`). No `initialize` handshake is required — the server
  is stateless.
- A short result is returned as one JSON body. Sending
  `Accept: text/event-stream` returns the response as SSE frames on the same POST.
- `MCP-Protocol-Version` is validated per request; the server negotiates the
  version on `initialize` (latest preferred, `2025-03-26` accepted).
- `GET` on the JSON-RPC endpoint returns `405` (there is no server-initiated
  stream in v1); `GET /mcp/:binding_id` returns binding metadata for back-compat.
- **Scope: tools only.** Resources and prompts are intentionally out of v1 — there
  are no stub methods.

## Endpoints

| Endpoint | Purpose |
|---|---|
| `POST /mcp/:binding_id` | JSON-RPC against one binding (local toolset or upstream proxy). |
| `GET /mcp/:binding_id` | Binding metadata. |
| `GET,POST /mcp` | **Aggregate** — every binding the caller's scopes allow, tools namespaced `{binding_id}:{tool}`. |

## Bindings

Two kinds, managed via `POST /v1/mcp/bindings` (admin):

- `local_toolset` — exposes a Polaris toolset (store-backed tool definitions run
  by the local tool registry).
- `upstream_proxy` — a remote MCP server. For per-binding routes Polaris forwards
  transparently (header allow-list, caller credentials stripped, binding headers
  injected). In the aggregate, Polaris acts as an MCP **client**, calling the
  upstream's `tools/list`/`tools/call` and namespacing its tools. A version
  mismatch falls back to the legacy `2025-03-26` revision.

## Authentication

```yaml
runtime:
  mcp:
    enabled: true
    auth: [api_key]          # api_key | oauth (or both)
    oauth:
      resource_uri: https://gateway.example.com/mcp
      authorization_servers: [https://auth.example.com]
      jwks_cache_ttl: 1h
```

- **api_key** (default) — the standard Polaris API-key / virtual-key auth, with
  `allowed_mcp_bindings` / `allowed_toolsets` scopes.
- **oauth** — an OAuth 2.1 resource server (RFC 9728). Bearer JWTs are validated
  against the authorization servers' JWKS (RS/ES signatures, `iss` allow-list,
  `aud` == `resource_uri`, required expiry; JWKS is cached, rotated, and
  stale-grace-tolerant). `GET /.well-known/oauth-protected-resource` advertises the
  metadata; a `401` carries `WWW-Authenticate: Bearer resource_metadata="…"`.
  Downstream tokens are **never** forwarded upstream.

When both are configured, a JWT bearer is validated via OAuth and anything else
falls through to the API-key path.

### OAuth scope → Polaris scope mapping

| Token scope | Grants |
|---|---|
| `mcp` or `mcp:*` | all bindings + toolsets |
| `mcp:binding:{id}` | that binding |
| `mcp:toolset:{id}` | that toolset |

## Governance

Every `tools/call` is authenticated, scope-checked, budget-checked, and metered
(`polaris_mcp_requests_total`, `polaris_tool_invocations_total`). Request/response
sizes are capped (4 MiB request, 1 MiB tool result → truncation).
