# Polaris Configuration

`config/polaris.yaml` is the local-development default. `config/polaris.example.yaml` is the full reference file that documents the intended production topology and the full provider catalog described in `BLUEPRINT.md`.

## Precedence

Configuration is resolved in this order:

1. CLI flags
2. Environment variables
3. YAML config
4. Built-in defaults

Supported CLI flags from the blueprint:

- `--config <path>`
- `--port <port>`
- `--log-level <level>`

## File Roles

- `config/polaris.yaml`: single-binary development defaults, centered on SQLite and in-memory cache.
- `config/polaris.example.yaml`: full reference config for real deployments and future phases.

## Sections

- `server`: listener address and request/shutdown timeouts.
- `auth`: auth mode plus static or admin key hashes.
- `store`: backing database driver, DSN, and async log writer tuning.
- `cache`: rate limiting and response-cache settings.
- `providers`: provider credentials, base URLs, retry policy, and model catalog.
- `routing`: alias definitions and provider failover order.
- `observability`: metrics and structured logging settings.

## Secret Handling

- Provider credentials must come from environment variables via `${VAR_NAME}` references.
- Do not commit plaintext API keys, admin keys, TLS material, or local `.env` files.
- Static and admin gateway keys must be stored as hashes, not plaintext values.

## Current Phase Guidance

Phase 1 only requires the pieces needed for chat routing and gateway fundamentals:

- `auth`
- `store`
- `cache`
- `providers.openai`
- `providers.anthropic`
- `routing`
- `observability`

The reference config already includes future-phase providers so the target model catalog is visible from the start.
