# @polaris/sdk

Official TypeScript SDK for the [Polaris](https://github.com/JiaCheng2004/Polaris)
AI gateway — one typed, OpenAI-compatible client for chat, embeddings, images,
audio, and more.

- **Dependency-free runtime** — built on the platform `fetch`; nothing to install
  beyond this package.
- **Typed from the contract** — request/response types are generated from the
  Polaris OpenAPI spec.
- **First-class streaming** — a hand-written SSE decoder tolerant of keep-alives.
- **ESM + CJS**, Node ≥ 18 and modern browsers.

## Install

```bash
npm install @polaris/sdk
```

## Usage

```ts
import { PolarisClient } from '@polaris/sdk';

const client = new PolarisClient({
  baseURL: 'http://localhost:8080',
  apiKey: process.env.POLARIS_API_KEY,
});

// Chat completion
const res = await client.createChatCompletion({
  model: 'openai/gpt-4o',
  messages: [{ role: 'user', content: 'Say hello in one word.' }],
});
console.log(res.choices[0]?.message.content);

// Streaming
const stream = await client.streamChatCompletion({
  model: 'openai/gpt-4o',
  messages: [{ role: 'user', content: 'Count to five.' }],
  stream: true,
});
for await (const chunk of stream) {
  process.stdout.write(chunk.choices[0]?.delta.content ?? '');
}

// Embeddings
const emb = await client.createEmbedding({
  model: 'openai/text-embedding-3-small',
  input: 'The quick brown fox.',
});
console.log(emb.data[0]?.embedding);
```

## Errors

Every non-2xx response throws an `APIError` with the OpenAI-compatible envelope
fields:

```ts
import { APIError } from '@polaris/sdk';

try {
  await client.createChatCompletion({ model: 'openai/gpt-4o', messages: [] });
} catch (err) {
  if (err instanceof APIError) {
    console.error(err.status, err.type, err.code, err.requestId);
  }
}
```

## Methods

`createChatCompletion`, `streamChatCompletion`, `createEmbedding`, `countTokens`,
`createTranslation`, `createImageGeneration`, `createSpeech`, `listModels`. Every
request/response type is exported (generated from the OpenAPI contract).

## Development

```bash
npm install
npm run generate   # regenerate types from ../../spec/openapi/polaris.v1.yaml
npm run typecheck
npm test
npm run build
```

## License

Apache-2.0
