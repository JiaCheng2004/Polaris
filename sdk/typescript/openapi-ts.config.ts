import { defineConfig } from '@hey-api/openapi-ts';

// Generate TypeScript types only from the enriched Polaris OpenAPI contract.
// The runtime (client, SSE streaming, errors) is hand-written in src/ so it can
// own SSE decoding and stay dependency-free — generators do not emit streaming.
export default defineConfig({
  input: '../../spec/openapi/polaris.v1.yaml',
  output: {
    path: 'src/generated',
    postProcess: [],
  },
  plugins: ['@hey-api/typescript'],
});
