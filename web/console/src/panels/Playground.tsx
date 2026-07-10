import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listModels } from "../api/resources";
import { streamChat, type ChatMessage } from "../api/sse";
import { useConn } from "../state/connection";
import { Field, KeyVal, PageHeader, Panel } from "../components/ui";

interface RunMeta {
  resolvedModel?: string;
  resolvedProvider?: string;
  fallback?: string;
  promptTokens?: number;
  completionTokens?: number;
}

export function Playground() {
  const conn = useConn();
  const models = useQuery({ queryKey: ["models", conn.baseURL], queryFn: () => listModels(conn) });
  const chatModels = (models.data ?? []).filter((m) => m.modality === "chat" || m.capability_flags?.chat);

  const [model, setModel] = useState("");
  const [system, setSystem] = useState("");
  const [prompt, setPrompt] = useState("Say hello in one sentence.");
  const [temperature, setTemperature] = useState("0.7");
  const [output, setOutput] = useState("");
  const [meta, setMeta] = useState<RunMeta>({});
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  // Select the first chat model once the catalog loads.
  useEffect(() => {
    const first = chatModels[0];
    if (!model && first) setModel(first.id);
  }, [model, chatModels]);

  const run = async () => {
    if (!model || !prompt) return;
    setRunning(true);
    setError(null);
    setOutput("");
    setMeta({});
    const controller = new AbortController();
    abortRef.current = controller;
    const messages: ChatMessage[] = [];
    if (system.trim()) messages.push({ role: "system", content: system });
    messages.push({ role: "user", content: prompt });
    try {
      let acc = "";
      for await (const ev of streamChat(conn, { model, messages, temperature: Number(temperature), signal: controller.signal })) {
        if (ev.delta) {
          acc += ev.delta;
          setOutput(acc);
        }
        if (ev.usage || ev.finishReason) {
          setMeta({
            resolvedModel: ev.headers.get("X-Polaris-Resolved-Model") || undefined,
            resolvedProvider: ev.headers.get("X-Polaris-Resolved-Provider") || undefined,
            fallback: ev.headers.get("X-Polaris-Fallback") || undefined,
            promptTokens: ev.usage?.prompt_tokens,
            completionTokens: ev.usage?.completion_tokens,
          });
        }
      }
    } catch (e) {
      if ((e as Error).name !== "AbortError") setError((e as Error).message);
    } finally {
      setRunning(false);
      abortRef.current = null;
    }
  };

  const stop = () => abortRef.current?.abort();
  const hasResult = meta.resolvedModel !== undefined || meta.promptTokens !== undefined;

  return (
    <div>
      <PageHeader title="Playground" />

      <div className="grid">
        <Panel className="col-8" title="Prompt" reveal>
          <Field label="System">
            <textarea
              className="textarea"
              value={system}
              onChange={(e) => setSystem(e.target.value)}
              placeholder="Optional system prompt"
              style={{ minHeight: 72 }}
            />
          </Field>
          <Field label="Message">
            <textarea
              className="textarea"
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              style={{ minHeight: 148 }}
            />
          </Field>
        </Panel>

        <Panel className="col-4" title="Run" reveal>
          <Field label="Model">
            <select className="select" value={model} onChange={(e) => setModel(e.target.value)}>
              {chatModels.length === 0 && <option value="">No chat models</option>}
              {chatModels.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.id}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Temperature">
            <input
              className="input mono"
              type="number"
              step="0.1"
              min="0"
              max="2"
              value={temperature}
              onChange={(e) => setTemperature(e.target.value)}
            />
          </Field>
          <div style={{ display: "flex", gap: 8, marginTop: 4 }}>
            <button className="btn btn-primary btn-sm" disabled={!model || running} onClick={() => void run()} style={{ flex: 1, justifyContent: "center" }}>
              {running ? "Streaming…" : "Send"}
            </button>
            {running && (
              <button className="btn btn-sm" onClick={stop}>
                Stop
              </button>
            )}
          </div>

          {hasResult && (
            <div style={{ marginTop: 18, borderTop: "1px solid var(--border)", paddingTop: 12 }}>
              <div className="meta" style={{ marginBottom: 6 }}>Result</div>
              <KeyVal k="provider">{meta.resolvedProvider ?? "n/a"}</KeyVal>
              <KeyVal k="model">{meta.resolvedModel ?? "n/a"}</KeyVal>
              {meta.fallback && (
                <KeyVal k="fallback">
                  <span style={{ color: "var(--status-warning)" }}>{meta.fallback}</span>
                </KeyVal>
              )}
              <KeyVal k="prompt tokens">{meta.promptTokens ?? "n/a"}</KeyVal>
              <KeyVal k="completion tokens">{meta.completionTokens ?? "n/a"}</KeyVal>
            </div>
          )}
        </Panel>

        <Panel className="col-12" title="Response" reveal>
          {error && (
            <div role="alert" style={{ color: "var(--danger)", fontSize: 13, marginBottom: 10 }}>
              {error}
            </div>
          )}
          <pre
            className="mono"
            style={{
              whiteSpace: "pre-wrap",
              wordBreak: "break-word",
              fontSize: 13,
              lineHeight: 1.6,
              minHeight: 220,
              margin: 0,
              color: "var(--text-1)",
            }}
          >
            {output}
          </pre>
        </Panel>
      </div>
    </div>
  );
}
