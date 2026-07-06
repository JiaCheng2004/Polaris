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

The Go SDK lives at `github.com/JiaCheng2004/Polaris/pkg/client`. Standalone,
runnable programs are in `go/`:

```bash
export POLARIS_API_KEY=sk-...
go run ./examples/go/chat         # one chat completion
go run ./examples/go/stream       # streaming chat completion (SSE)
go run ./examples/go/embeddings   # an embedding vector
```

Godoc examples for every surface also live in `pkg/client/example_test.go` and
render on [pkg.go.dev](https://pkg.go.dev/github.com/JiaCheng2004/Polaris/pkg/client).

## TypeScript SDK

The TypeScript SDK is published as `@polaris/sdk` (source in `sdk/typescript/`).
`typescript/chat.ts` is a runnable consumer example:

```bash
npm install @polaris/sdk
export POLARIS_API_KEY=sk-...
npx tsx examples/typescript/chat.ts
```

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
