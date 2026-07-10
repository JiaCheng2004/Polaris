import type { ErrorEnvelope } from "./types";

export interface Conn {
  baseURL: string;
  token: string;
}

export class ApiError extends Error {
  status: number;
  type: string;
  code?: string;
  constructor(status: number, message: string, type = "error", code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.type = type;
    this.code = code;
  }
}

function joinURL(base: string, path: string): string {
  return base.replace(/\/+$/, "") + path;
}

export function authHeaders(conn: Conn): Record<string, string> {
  const h: Record<string, string> = {};
  if (conn.token) h["Authorization"] = `Bearer ${conn.token}`;
  return h;
}

async function parseError(res: Response): Promise<ApiError> {
  let message = `${res.status} ${res.statusText}`;
  let type = "error";
  let code: string | undefined;
  try {
    const body = (await res.json()) as ErrorEnvelope;
    if (body?.error) {
      message = body.error.message || message;
      type = body.error.type || type;
      code = body.error.code;
    }
  } catch {
    /* non-JSON body */
  }
  return new ApiError(res.status, message, type, code);
}

export interface RequestOpts {
  method?: string;
  query?: Record<string, string | number | boolean | undefined | null>;
  body?: unknown;
  signal?: AbortSignal;
}

function withQuery(path: string, query?: RequestOpts["query"]): string {
  if (!query) return path;
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v !== undefined && v !== null && v !== "") qs.set(k, String(v));
  }
  const s = qs.toString();
  return s ? `${path}?${s}` : path;
}

export async function request<T>(conn: Conn, path: string, opts: RequestOpts = {}): Promise<T> {
  const url = joinURL(conn.baseURL, withQuery(path, opts.query));
  const headers: Record<string, string> = { ...authHeaders(conn) };
  let body: string | undefined;
  if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(opts.body);
  }
  const res = await fetch(url, {
    method: opts.method ?? "GET",
    headers,
    body,
    signal: opts.signal,
  });
  if (!res.ok) throw await parseError(res);
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

/** Raw fetch (for SSE streaming); caller reads res.body. */
export async function rawFetch(conn: Conn, path: string, opts: RequestOpts = {}): Promise<Response> {
  const url = joinURL(conn.baseURL, withQuery(path, opts.query));
  const headers: Record<string, string> = { ...authHeaders(conn) };
  let body: string | undefined;
  if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(opts.body);
  }
  const res = await fetch(url, {
    method: opts.method ?? "POST",
    headers,
    body,
    signal: opts.signal,
  });
  if (!res.ok) throw await parseError(res);
  return res;
}
