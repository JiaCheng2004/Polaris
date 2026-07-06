# Changelog

All notable changes to Polaris are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Polaris follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0]

Initial public release: a stateless, config-driven, multi-modality AI gateway
that presents one OpenAI-compatible API across many providers.

### Gateway

- Unified `/v1` API for chat, responses, messages, embeddings, images, video,
  voice, audio sessions, transcription, translation, notes, podcasts, and music.
- 24 provider integrations across chat, embeddings, image, audio, video, and
  music, with `provider/model` naming, aliases, and family-aware routing.
- Auth modes: none, static keys, external signed headers, virtual keys, and a
  control plane for projects, keys, policies, budgets, tools, and MCP bindings.
- SQLite and PostgreSQL stores; in-memory and Redis caches; Prometheus metrics,
  structured logs, and optional OpenTelemetry tracing (incl. GenAI semconv).

### Reliability

- Circuit breaking, health-aware adaptive routing, jittered retries with
  `Retry-After`, load shedding, request hedging, and idempotency keys.
- Graceful shutdown with WebSocket drain and long-lived SSE survival; durable,
  never-block-the-response usage and audit logging.

### Frontier features

- **Guardrails** engine: PII, secrets, and prompt-injection detection plus
  webhook and LLM-judge remote detectors, with observe/redact/block actions on
  both unary and streaming responses.
- **Semantic cache**: exact (SHA-256) + embedding (cosine) layers with
  security-grade project/model/settings/epoch isolation and graceful degradation.
- **MCP-native gateway**: stateless streamable-HTTP Model Context Protocol server
  and upstream client with tool aggregation and an OAuth 2.1 resource server.

### SDKs & tooling

- Go SDK (`pkg/client`) covering every surface, with package docs and examples.
- Signed, multi-arch container images (cosign + SLSA provenance + SBOM) and
  signed release binaries via GoReleaser.

### License

- Released under the Apache License 2.0.
