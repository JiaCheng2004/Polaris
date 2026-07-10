import { describe, expect, it } from "vitest";
import { sseLines } from "../src/api/sse";

function responseFrom(chunks: string[]): Response {
  const enc = new TextEncoder();
  let i = 0;
  const stream = new ReadableStream<Uint8Array>({
    pull(controller) {
      if (i < chunks.length) controller.enqueue(enc.encode(chunks[i++]!));
      else controller.close();
    },
  });
  return new Response(stream);
}

async function collect(res: Response): Promise<string[]> {
  const out: string[] = [];
  for await (const line of sseLines(res)) out.push(line);
  return out;
}

describe("SSE decoder", () => {
  it("accumulates data lines and stops on [DONE]", async () => {
    const res = responseFrom(["data: a\n\n", "data: b\n\n", "data: [DONE]\n\n", "data: c\n\n"]);
    expect(await collect(res)).toEqual(["a", "b"]);
  });

  it("tolerates CRLF and keep-alive comments and split chunk boundaries", async () => {
    const res = responseFrom(["data: hel", "lo\r\n", ": ping\r\n", "data: world\r\n"]);
    expect(await collect(res)).toEqual(["hello", "world"]);
  });
});
