# Examples

Runnable examples for using Polaris.

## curl

`curl/` contains a cookbook of raw HTTP requests against a running gateway
(default `http://localhost:8080`). Set `POLARIS_KEY` to your API key.

```bash
export POLARIS_KEY=sk-...
./curl/chat.sh
```

## Go SDK

The Go SDK lives at `github.com/JiaCheng2004/Polaris/pkg/client`. Runnable
examples for every surface are in `pkg/client/example_test.go` and render on
[pkg.go.dev](https://pkg.go.dev/github.com/JiaCheng2004/Polaris/pkg/client).

## Configuration

Ready-to-copy configuration lives in `config/polaris.example.yaml`. The feature
guides show focused fragments:

- Routing & failover — `docs/CONFIGURATION.md`
- Guardrails — `docs/GUARDRAILS.md`
- Semantic cache — `docs/SEMANTIC_CACHE.md`
- MCP gateway — `docs/MCP.md`

## Docker Compose

`deployments/docker-compose.yml` brings up Polaris with Postgres and Redis for a
production-shaped local stack.
