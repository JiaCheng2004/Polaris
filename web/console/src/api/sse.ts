import { rawFetch, type Conn } from "./http";

// Chunk-boundary-safe SSE reader: accumulates `data:` lines, tolerates CRLF and
// `:` keep-alive/ping comments, and terminates on `[DONE]`. Mirrors the approach
// in the official @polaris/sdk streaming decoder.
export async function* sseLines(res: Response, signal?: AbortSignal): AsyncGenerator<string> {
  const reader = res.body?.getReader();
  if (!reader) return;
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    while (true) {
      if (signal?.aborted) return;
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let idx: number;
      while ((idx = buffer.indexOf("\n")) >= 0) {
        const line = buffer.slice(0, idx).replace(/\r$/, "");
        buffer = buffer.slice(idx + 1);
        if (!line || line.startsWith(":")) continue; // blank or keep-alive
        if (!line.startsWith("data:")) continue;
        const data = line.slice(5).trim();
        if (data === "[DONE]") return;
        yield data;
      }
    }
  } finally {
    reader.releaseLock();
  }
}

export interface ChatMessage {
  role: "system" | "user" | "assistant";
  content: string;
}

export interface StreamChatOptions {
  model: string;
  messages: ChatMessage[];
  temperature?: number;
  signal?: AbortSignal;
}

export interface ChatStreamEvent {
  delta: string;
  finishReason?: string | null;
  usage?: { prompt_tokens?: number; completion_tokens?: number; total_tokens?: number };
  headers: Headers;
}

// Streams a chat completion, yielding text deltas plus the final usage frame.
// The gateway surfaces the resolved provider/model in X-Polaris-Resolved-* headers.
export async function* streamChat(
  conn: Conn,
  opts: StreamChatOptions
): AsyncGenerator<ChatStreamEvent> {
  const res = await rawFetch(conn, "/v1/chat/completions", {
    method: "POST",
    body: {
      model: opts.model,
      messages: opts.messages,
      temperature: opts.temperature,
      stream: true,
      stream_options: { include_usage: true },
    },
    signal: opts.signal,
  });
  const headers = res.headers;
  for await (const data of sseLines(res, opts.signal)) {
    let parsed: {
      choices?: { delta?: { content?: string }; finish_reason?: string | null }[];
      usage?: ChatStreamEvent["usage"];
    };
    try {
      parsed = JSON.parse(data);
    } catch {
      continue;
    }
    const choice = parsed.choices?.[0];
    yield {
      delta: choice?.delta?.content ?? "",
      finishReason: choice?.finish_reason ?? null,
      usage: parsed.usage,
      headers,
    };
  }
}
