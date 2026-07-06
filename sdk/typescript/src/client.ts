import type {
  ChatCompletion,
  ChatCompletionRequest,
  EmbeddingRequest,
  EmbeddingResponse,
  ErrorEnvelope,
  ImageGenerationRequest,
  ImageResponse,
  ModelListResponse,
  SpeechRequest,
  TokenCountRequest,
  TokenCountResponse,
  TranslationRequest,
} from './generated/types.gen';
import { APIError } from './errors';
import { Stream } from './streaming';
import type { ChatCompletionChunk } from './types';

/** PolarisClientOptions configures a {@link PolarisClient}. */
export interface PolarisClientOptions {
  /** Bearer API key sent on every request. */
  apiKey?: string;
  /** Gateway base URL (default `http://localhost:8080`). */
  baseURL?: string;
  /** Per-request timeout in milliseconds (default 60000). */
  timeoutMs?: number;
  /** Override the `fetch` implementation (e.g. for Node < 18 or testing). */
  fetch?: typeof globalThis.fetch;
  /** Extra headers merged into every request. */
  defaultHeaders?: Record<string, string>;
}

const DEFAULT_BASE_URL = 'http://localhost:8080';
const DEFAULT_TIMEOUT_MS = 60_000;

/**
 * PolarisClient is a typed client for the Polaris AI gateway. It is safe to
 * share across concurrent calls.
 *
 * ```ts
 * const client = new PolarisClient({ apiKey: process.env.POLARIS_API_KEY });
 * const res = await client.createChatCompletion({
 *   model: 'openai/gpt-4o',
 *   messages: [{ role: 'user', content: 'Hello' }],
 * });
 * ```
 */
export class PolarisClient {
  /** The resolved gateway base URL (trailing slashes stripped). */
  readonly baseURL: string;
  private readonly apiKey: string | undefined;
  private readonly timeoutMs: number;
  private readonly fetchImpl: typeof globalThis.fetch;
  private readonly defaultHeaders: Record<string, string>;

  constructor(options: PolarisClientOptions = {}) {
    this.baseURL = (options.baseURL ?? DEFAULT_BASE_URL).replace(/\/+$/, '');
    this.apiKey = options.apiKey;
    this.timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
    const fetchImpl = options.fetch ?? globalThis.fetch;
    if (!fetchImpl) {
      throw new Error('global fetch is unavailable; pass options.fetch (Node < 18)');
    }
    this.fetchImpl = fetchImpl;
    this.defaultHeaders = options.defaultHeaders ?? {};
  }

  /** Create a chat completion (non-streaming). */
  createChatCompletion(body: ChatCompletionRequest): Promise<ChatCompletion> {
    return this.postJSON('/v1/chat/completions', { ...body, stream: false });
  }

  /** Open a streaming chat completion; iterate the returned {@link Stream} for deltas. */
  async streamChatCompletion(body: ChatCompletionRequest): Promise<Stream<ChatCompletionChunk>> {
    const response = await this.request('POST', '/v1/chat/completions', {
      body: { ...body, stream: true },
      accept: 'text/event-stream',
    });
    return new Stream<ChatCompletionChunk>(response, (data) => JSON.parse(data) as ChatCompletionChunk);
  }

  /** Create embeddings for one or more inputs. */
  createEmbedding(body: EmbeddingRequest): Promise<EmbeddingResponse> {
    return this.postJSON('/v1/embeddings', body);
  }

  /** Count the tokens a prospective request would consume. */
  countTokens(body: TokenCountRequest): Promise<TokenCountResponse> {
    return this.postJSON('/v1/tokens/count', body);
  }

  /** Translate text into a target language. */
  createTranslation<T = unknown>(body: TranslationRequest): Promise<T> {
    return this.postJSON('/v1/translations', body);
  }

  /** Generate images from a prompt. */
  createImageGeneration(body: ImageGenerationRequest): Promise<ImageResponse> {
    return this.postJSON('/v1/images/generations', body);
  }

  /** Synthesize speech; resolves to the raw audio bytes. */
  async createSpeech(body: SpeechRequest): Promise<ArrayBuffer> {
    const response = await this.request('POST', '/v1/audio/speech', { body, accept: 'audio/*' });
    return response.arrayBuffer();
  }

  /** List the models the gateway exposes. */
  listModels(): Promise<ModelListResponse> {
    return this.getJSON('/v1/models');
  }

  // --- internals -----------------------------------------------------------

  private async postJSON<T>(path: string, body: unknown): Promise<T> {
    const response = await this.request('POST', path, { body });
    return (await response.json()) as T;
  }

  private async getJSON<T>(path: string): Promise<T> {
    const response = await this.request('GET', path);
    return (await response.json()) as T;
  }

  private async request(
    method: string,
    path: string,
    opts: { body?: unknown; accept?: string } = {},
  ): Promise<Response> {
    const headers: Record<string, string> = {
      Accept: opts.accept ?? 'application/json',
      ...this.defaultHeaders,
    };
    if (this.apiKey) {
      headers['Authorization'] = `Bearer ${this.apiKey}`;
    }
    let payload: string | undefined;
    if (opts.body !== undefined) {
      headers['Content-Type'] = 'application/json';
      payload = JSON.stringify(opts.body);
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);
    let response: Response;
    try {
      response = await this.fetchImpl(`${this.baseURL}${path}`, {
        method,
        headers,
        body: payload,
        signal: controller.signal,
      });
    } finally {
      clearTimeout(timer);
    }

    if (!response.ok) {
      const requestId = response.headers.get('x-request-id') ?? undefined;
      const text = await response.text();
      let envelope: ErrorEnvelope | undefined;
      try {
        envelope = JSON.parse(text) as ErrorEnvelope;
      } catch {
        // non-JSON error body; APIError falls back to a generic message
      }
      throw new APIError(response.status, envelope, requestId);
    }
    return response;
  }
}
