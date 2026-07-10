import { useQuery } from "@tanstack/react-query";
import { getReady } from "../api/resources";
import { useConn } from "../state/connection";
import { EmptyState, ErrorNote, PageHeader, Panel, Spinner, StatusChip } from "../components/ui";
import { Meter, fmtInt, fmtMs, fmtPct } from "../viz/charts";
import type { HealthView } from "../api/types";
import { useRef } from "react";
import { useReveal } from "../motion/hooks";

function ProviderCard({ h }: { h: HealthView }) {
  const errTone = h.error_rate >= 0.5 ? "var(--status-critical)" : h.error_rate >= 0.2 ? "var(--status-warning)" : "var(--text-1)";
  return (
    <div className="col-4" data-reveal="">
      <div className="panel" style={{ padding: "16px" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 12 }}>
          <span style={{ fontWeight: 600, fontSize: 14 }}>{h.provider}</span>
          <StatusChip state={h.breaker} />
        </div>
        <Meter value={h.health_score} max={1} label="Health" valueText={fmtPct(h.health_score)} tone={h.health_score >= 0.8 ? "var(--status-good)" : h.health_score >= 0.5 ? "var(--status-warning)" : "var(--status-critical)"} />
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "10px 16px", marginTop: 14 }}>
          <Metric label="Error rate" value={fmtPct(h.error_rate)} tone={errTone} />
          <Metric label="EWMA latency" value={fmtMs(h.ewma_latency_ms)} />
          <Metric label="In-flight" value={fmtInt(h.inflight)} />
          <Metric label="Samples" value={fmtInt(h.samples)} />
        </div>
      </div>
    </div>
  );
}

function Metric({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div>
      <div className="meta">{label}</div>
      <div className="num" style={{ fontSize: 15, color: tone ?? "var(--text-1)" }}>
        {value}
      </div>
    </div>
  );
}

export function Providers() {
  const conn = useConn();
  const scope = useRef<HTMLDivElement>(null);
  const q = useQuery({
    queryKey: ["ready", conn.baseURL],
    queryFn: () => getReady(conn),
    refetchInterval: 5000,
  });
  const providers = q.data?.reliability ?? [];
  useReveal(scope, [providers.length]);

  return (
    <div ref={scope}>
      <PageHeader title="Providers & Health" />

      {q.isLoading && <Spinner />}
      {q.isError && <ErrorNote message={(q.error as Error).message} />}

      {q.data && (
        <div className="grid">
          {providers.length === 0 ? (
            <div className="col-12">
              <Panel>
                <EmptyState
                  title="No provider health yet"
                  hint="Health is accumulated from real requests. Send traffic (try the Playground) and this fills in. Store/cache reachable; providers configured: {count}."
                />
                <div className="meta" style={{ textAlign: "center" }}>
                  store {q.data.store ?? "?"} · cache {q.data.cache ?? "?"} · providers {q.data.providers ?? 0}
                </div>
              </Panel>
            </div>
          ) : (
            providers.map((h) => <ProviderCard key={h.provider} h={h} />)
          )}
        </div>
      )}
    </div>
  );
}
