// Runnable example: a chat completion via the Polaris TypeScript SDK.
//
//   npm install @polaris/sdk
//   POLARIS_API_KEY=sk-... npx tsx chat.ts
//
// Environment:
//   POLARIS_BASE_URL  gateway URL (default http://localhost:8080)
//   POLARIS_API_KEY   API key for the gateway
//   POLARIS_MODEL     model as "provider/model" (default openai/gpt-4o)
import { PolarisClient } from '@polaris/sdk';

const client = new PolarisClient({
  baseURL: process.env.POLARIS_BASE_URL ?? 'http://localhost:8080',
  apiKey: process.env.POLARIS_API_KEY,
});

const res = await client.createChatCompletion({
  model: process.env.POLARIS_MODEL ?? 'openai/gpt-4o',
  messages: [
    { role: 'system', content: 'You are concise.' },
    { role: 'user', content: 'Say hello in one word.' },
  ],
});

console.log(res.choices[0]?.message.content);
