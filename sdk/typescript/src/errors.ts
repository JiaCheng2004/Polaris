import type { ErrorEnvelope } from './generated/types.gen';

/**
 * APIError is thrown for any non-2xx response from the gateway. It carries the
 * OpenAI-compatible error envelope fields ({@link type}, {@link code},
 * {@link param}) plus the HTTP status and request id for debugging.
 */
export class APIError extends Error {
  /** HTTP status code of the failed response. */
  readonly status: number;
  /** Machine-readable error category, e.g. `rate_limit_error`, `model_not_found`. */
  readonly type: string;
  /** Optional finer-grained error code. */
  readonly code?: string;
  /** Optional offending request parameter. */
  readonly param?: string;
  /** The gateway's `x-request-id`, if present, for support/correlation. */
  readonly requestId?: string;

  constructor(status: number, envelope: ErrorEnvelope | undefined, requestId?: string) {
    const err = envelope?.error;
    super(err?.message ?? `Polaris request failed with HTTP ${status}`);
    this.name = 'APIError';
    this.status = status;
    this.type = err?.type ?? 'internal_error';
    this.code = err?.code;
    this.param = err?.param;
    this.requestId = requestId;
  }
}
