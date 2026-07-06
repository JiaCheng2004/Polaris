import type { Usage } from './generated/types.gen';

// Streaming chat shapes. The gateway's streaming responses are OpenAI-compatible
// SSE and are not part of the request/response OpenAPI schemas, so they are
// defined here by hand.

/** ChatCompletionChunk is one streamed chunk of a chat completion. */
export interface ChatCompletionChunk {
  id: string;
  object: string;
  created: number;
  model: string;
  choices: ChatCompletionChunkChoice[];
  /** Present on the final chunk. */
  usage?: Usage;
}

/** ChatCompletionChunkChoice is one choice within a streamed chunk. */
export interface ChatCompletionChunkChoice {
  index: number;
  delta: ChatCompletionChunkDelta;
  finish_reason?: string | null;
}

/** ChatCompletionChunkDelta is the incremental delta for a streamed choice. */
export interface ChatCompletionChunkDelta {
  role?: string;
  content?: string;
}
