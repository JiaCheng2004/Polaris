/**
 * Stream is an async-iterable over a Server-Sent Events response. It decodes the
 * SSE wire format (multi-line `data:` accumulation, CRLF, comment/`ping`
 * keep-alives) and terminates on the OpenAI-compatible `data: [DONE]` sentinel.
 *
 * ```ts
 * const stream = await client.streamChatCompletion({ ... });
 * for await (const chunk of stream) {
 *   process.stdout.write(chunk.choices[0]?.delta.content ?? '');
 * }
 * ```
 */
export class Stream<T> implements AsyncIterable<T> {
  constructor(
    private readonly response: Response,
    private readonly parse: (data: string) => T,
  ) {}

  async *[Symbol.asyncIterator](): AsyncIterator<T> {
    const body = this.response.body;
    if (!body) {
      return;
    }
    const reader = body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let dataLines: string[] = [];

    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        buffer += decoder.decode(value, { stream: true });

        let newline: number;
        while ((newline = buffer.indexOf('\n')) >= 0) {
          let line = buffer.slice(0, newline);
          buffer = buffer.slice(newline + 1);
          if (line.endsWith('\r')) {
            line = line.slice(0, -1);
          }
          if (line === '') {
            // Blank line = end of an event: dispatch accumulated data.
            if (dataLines.length > 0) {
              const data = dataLines.join('\n');
              dataLines = [];
              if (data === '[DONE]') {
                return;
              }
              yield this.parse(data);
            }
            continue;
          }
          if (line.startsWith(':')) {
            continue; // comment / keep-alive (e.g. Anthropic `: ping`)
          }
          if (line.startsWith('data:')) {
            dataLines.push(line.slice(5).replace(/^ /, ''));
          }
          // event:/id:/retry: fields are intentionally ignored.
        }
      }
      // A trailing event with no final blank line.
      if (dataLines.length > 0) {
        const data = dataLines.join('\n');
        if (data !== '[DONE]') {
          yield this.parse(data);
        }
      }
    } finally {
      reader.releaseLock();
    }
  }

  /** Cancels the underlying response body, aborting the stream early. */
  async close(): Promise<void> {
    try {
      await this.response.body?.cancel();
    } catch {
      // already closed
    }
  }
}
