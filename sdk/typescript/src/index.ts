/**
 * Official TypeScript SDK for the Polaris AI gateway.
 *
 * @packageDocumentation
 */
export { PolarisClient } from './client';
export type { PolarisClientOptions } from './client';
export { APIError } from './errors';
export { Stream } from './streaming';
export type {
  ChatCompletionChunk,
  ChatCompletionChunkChoice,
  ChatCompletionChunkDelta,
} from './types';

// Re-export every generated request/response type from the OpenAPI contract.
export type * from './generated/types.gen';
