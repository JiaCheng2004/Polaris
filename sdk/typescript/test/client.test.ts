import { describe, it, expect, vi } from 'vitest';
import { PolarisClient, APIError } from '../src/index';

type Fetch = typeof globalThis.fetch;

function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'content-type': 'application/json' },
    ...init,
  });
}

function sseResponse(frames: string[]): Response {
  const encoder = new TextEncoder();
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const frame of frames) {
        controller.enqueue(encoder.encode(frame));
      }
      controller.close();
    },
  });
  return new Response(stream, {
    status: 200,
    headers: { 'content-type': 'text/event-stream' },
  });
}

function chunkFrame(content: string): string {
  return `data: {"id":"1","object":"chat.completion.chunk","created":0,"model":"m","choices":[{"index":0,"delta":{"content":"${content}"}}]}\n\n`;
}

describe('PolarisClient', () => {
  it('createChatCompletion sends auth + body and parses the response', async () => {
    const fetchMock = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
      expect(String(url)).toBe('https://gw.test/v1/chat/completions');
      const headers = init?.headers as Record<string, string>;
      expect(headers['Authorization']).toBe('Bearer sk-test');
      expect(headers['Content-Type']).toBe('application/json');
      const body = JSON.parse(init?.body as string);
      expect(body.model).toBe('openai/gpt-4o');
      expect(body.stream).toBe(false);
      return jsonResponse({
        id: 'x',
        object: 'chat.completion',
        choices: [{ index: 0, message: { role: 'assistant', content: 'hi' }, finish_reason: 'stop' }],
        usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
      });
    });
    const client = new PolarisClient({
      apiKey: 'sk-test',
      baseURL: 'https://gw.test/',
      fetch: fetchMock as unknown as Fetch,
    });
    const res = await client.createChatCompletion({
      model: 'openai/gpt-4o',
      messages: [{ role: 'user', content: 'hi' }],
    });
    expect(res.choices[0]?.message).toBeDefined();
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it('streamChatCompletion yields deltas, tolerates ping, and stops at [DONE]', async () => {
    const frames = [chunkFrame('Hel'), ': ping\n\n', chunkFrame('lo'), 'data: [DONE]\n\n'];
    const fetchMock = vi.fn(async () => sseResponse(frames));
    const client = new PolarisClient({ baseURL: 'https://gw.test', fetch: fetchMock as unknown as Fetch });
    const stream = await client.streamChatCompletion({
      model: 'm',
      messages: [{ role: 'user', content: 'hi' }],
      stream: true,
    });
    let text = '';
    for await (const chunk of stream) {
      text += chunk.choices[0]?.delta.content ?? '';
    }
    expect(text).toBe('Hello');
  });

  it('decodes SSE events split across network chunk boundaries', async () => {
    const whole = chunkFrame('X') + 'data: [DONE]\n\n';
    const frames = [whole.slice(0, 40), whole.slice(40)]; // arbitrary mid-JSON split
    const fetchMock = vi.fn(async () => sseResponse(frames));
    const client = new PolarisClient({ baseURL: 'https://gw.test', fetch: fetchMock as unknown as Fetch });
    const stream = await client.streamChatCompletion({ model: 'm', messages: [], stream: true });
    let text = '';
    for await (const chunk of stream) {
      text += chunk.choices[0]?.delta.content ?? '';
    }
    expect(text).toBe('X');
  });

  it('throws APIError carrying the envelope fields on non-2xx', async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse(
        { error: { message: 'slow down', type: 'rate_limit_error', code: 'rate_limited' } },
        { status: 429, headers: { 'content-type': 'application/json', 'x-request-id': 'req-1' } },
      ),
    );
    const client = new PolarisClient({ baseURL: 'https://gw.test', fetch: fetchMock as unknown as Fetch });
    const err = await client.createChatCompletion({ model: 'm', messages: [] }).catch((e) => e);
    expect(err).toBeInstanceOf(APIError);
    expect(err).toMatchObject({
      status: 429,
      type: 'rate_limit_error',
      code: 'rate_limited',
      requestId: 'req-1',
    });
  });

  it('strips trailing slashes from baseURL', () => {
    const client = new PolarisClient({ baseURL: 'https://gw.test/v1///' });
    expect(client.baseURL).toBe('https://gw.test/v1');
  });
});
