# Polaris

**A stateless, config-driven AI gateway for routing one application across many
model providers and modalities.**

Polaris is a Go gateway that sits between your application and upstream AI
providers. Your app calls one stable, OpenAI-compatible API, while Polaris
handles provider credentials, model routing, failover, authentication, rate
limiting, usage logging, response caching, guardrails, and operational safety.

## Start here

- **[Configuration](CONFIGURATION.md)** — precedence, minimal config, `${ENV}`
  references.
- **[Authentication](AUTHENTICATION.md)** — auth modes and when to use each.
- **[API Reference](API_REFERENCE.md)** — the full `/v1` surface.

## Guides

- **[Guardrails](GUARDRAILS.md)** — PII/secrets/prompt-injection detection with
  observe → redact → block rollout.
- **[Semantic Cache](SEMANTIC_CACHE.md)** — exact + embedding cache with
  project isolation.
- **[MCP Gateway](MCP.md)** — stateless streamable-HTTP MCP with OAuth 2.1.
- **[Providers](PROVIDERS.md)** — the provider matrix and auth quirks.

## Reference

- **[Architecture](ARCHITECTURE.md)** — request lifecycle, layering, hot reload.
- **[Adding a Provider](ADDING_PROVIDER.md)** — the transport + adapter kit.

## Try it

```bash
docker run --rm -p 8080:8080 -e OPENAI_API_KEY=sk-... \
  ghcr.io/jiacheng2004/polaris:v1.0.0
```

Polaris is open source under the Apache-2.0 license.
