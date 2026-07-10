# Console

The Polaris Console is an optional web UI for operating a Polaris gateway. It is a
standalone single-page app under `web/console/` that talks to a gateway over its
`/v1` API and holds no state of its own. It is not part of the gateway binary and
is not required to run Polaris.

It complements Grafana rather than replacing it. Grafana owns aggregate time-series
metrics (traffic, latency percentiles, errors, throughput, cost). The console owns
management and drill-down: the control plane, the model catalog, live provider
health, usage and cost breakdowns, an audit trail, and a routing playground.

## Run it

```bash
cd web/console
npm ci
npm run dev      # http://localhost:5173
```

For a production build:

```bash
npm run build    # static assets in web/console/dist — serve from any static host
```

### Embedded in the gateway (optional)

The console can also be served **from the gateway binary itself** — handy for a
single-container deployment. This is opt-in and does not affect the default build:
the standard `make build` / image stays lean, pure-Go, and console-free. A binary
built with the `console` tag embeds the SPA and serves it on its own port when you
pass `--console`:

```bash
make build-console                       # builds the console, embeds it, compiles with -tags console
./bin/polaris --config ./config/polaris.yaml --console        # API on :8080, console on :8081
# ./bin/polaris ... --console --console-addr :9000            # override the console port
```

Or use the batteries-included image (`deployments/Dockerfile.console`):

```bash
docker build -f deployments/Dockerfile.console -t polaris-console .
docker run --rm -p 8080:8080 -p 8081:8081 polaris-console      # console at http://localhost:8081
```

The embedded console is served on a **separate listener** from the API and is an
admin surface, so in production put it behind your own TLS/reverse proxy and
network controls. Running the standard (non-`console`) binary with `--console`
prints a pointer to run the console standalone instead of failing.

Then open the console and connect:

- **Gateway base URL** — e.g. `http://localhost:8080`.
- **Admin bearer token** — the bootstrap admin key, or a virtual key created with
  `is_admin: true`. The console authenticates with `Authorization: Bearer` for both
  observability and control-plane calls.

The token is kept only in the browser tab's `sessionStorage` and is cleared when the
tab closes. Provider secrets are never stored or displayed; stored MCP upstream
headers are redacted in the UI.

## Getting an admin token

If this is a fresh gateway and you don't have an admin key yet, use the **bootstrap
admin key**. You choose the key; the gateway only ever stores its SHA-256 hash. No
Go toolchain is required — any machine with `openssl` can produce it:

```bash
# 1. Choose a strong admin key — this is the value you paste into the console. Keep it.
export POLARIS_ADMIN_KEY=$(openssl rand -hex 32)

# 2. Compute the hash to put in config (only the hash is ever persisted).
echo "sha256:$(printf '%s' "$POLARIS_ADMIN_KEY" | openssl dgst -sha256 -r | awk '{print $1}')"
```

Then start the gateway with `auth.mode: virtual_keys` and `control_plane.enabled: true`,
setting the printed `sha256:…` value as either the config field or the env var:

```yaml
runtime:
  auth:
    mode: virtual_keys
    bootstrap_admin_key_hash: sha256:…   # the value printed above
  control_plane:
    enabled: true
```

or `-e POLARIS_BOOTSTRAP_ADMIN_KEY_HASH=sha256:…`. Log into the console with
`$POLARIS_ADMIN_KEY`. Once in, mint a durable admin key under **Virtual Keys**
(`is_admin: true`) and use that going forward; the bootstrap key is only for
first access. (If you have a Go toolchain, `go run ./scripts/generate-key.go`
produces a key and its hash as an alternative.)

## What it shows

| Section | Source |
| --- | --- |
| Overview | admin usage snapshot + live `/ready` provider health |
| Usage & Cost | admin usage with per-model / provider / modality / status breakdowns and token economics (input, output, cached, cache-write) |
| Providers | live circuit-breaker, health score, error rate, EWMA latency, in-flight from `/ready` |
| Models | `/v1/models` catalog — families, aliases, capabilities, verification, routing |
| Projects, Virtual Keys, Policies, Budgets, Tools, Toolsets, MCP Bindings | control-plane create/list (the API is create-and-list; there is no edit, and only keys can be revoked) |
| Audit Log | admin audit-event listing |
| Playground | send a chat/embeddings request and watch the stream, resolved provider/model, and usage/cost |

Usage, Cost, and Audit require the gateway's admin analytics endpoints
(`GET /v1/admin/usage`, `GET /v1/admin/audit-events`) and `control_plane.enabled: true`.
Against an older gateway those sections stay empty while the rest continues to work.

## Design

Two independent controls in the top bar:

- **Theme** — light or dark (defaults to the OS preference).
- **Mode** — Standard (a clean, minimalist operator UI) or Advanced (a high-density
  tactical-telemetry style). Both work in either theme.

## CORS

For local development the gateway allows `localhost` origins out of the box, so no
change is needed. To serve the console from another origin in production, add that
origin to `runtime.server.cors.allowed_origins` in the gateway config — a
configuration change only, no code change. `Authorization`, `DELETE`, and the
`X-Polaris-*` response headers the console reads are already permitted.

## Security

The console requires an admin credential and can therefore issue and revoke keys and
create control-plane resources. Treat access to a connected console as equivalent to
holding the admin key. Serve it over HTTPS in production and restrict who can reach it.
