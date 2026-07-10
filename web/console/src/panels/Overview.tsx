import { useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { getAdminUsage, getReady, listModels } from "../api/resources";
import { useConn } from "../state/connection";
import { useTheme } from "../state/theme";
import { Panel, PageHeader, Readout, ReadoutGrid, StatusChip } from "../components/ui";
import { DataTable, type Column } from "../components/DataTable";
import { BarList, StatTile, TimeSeries, fmtInt, fmtMs, fmtPct, fmtUSD, type Series } from "../viz/charts";
import type { AdminUsageRow } from "../api/types";
import { useReveal } from "../motion/hooks";

const fmtFull = (n: number) => Intl.NumberFormat("en").format(Math.round(n));

function daysAgoISO(n: number): string {
  return new Date(Date.now() - n * 86400_000).toISOString();
}

/** Providers present in the catalog, with model counts, derived from model ids. */
function providersFromModels(ids: string[]): { name: string; models: number }[] {
  const counts = new Map<string, number>();
  for (const id of ids) {
    const slash = id.indexOf("/");
    if (slash <= 0) continue;
    const p = id.slice(0, slash);
    counts.set(p, (counts.get(p) ?? 0) + 1);
  }
  return [...counts.entries()].map(([name, models]) => ({ name, models })).sort((a, b) => a.name.localeCompare(b.name));
}

export function Overview() {
  const conn = useConn();
  const { mode } = useTheme();
  const advanced = mode === "advanced";
  const scope = useRef<HTMLDivElement>(null);

  const ready = useQuery({ queryKey: ["ready", conn.baseURL], queryFn: () => getReady(conn), refetchInterval: 8000, retry: false });
  const models = useQuery({ queryKey: ["models", conn.baseURL], queryFn: () => listModels(conn), retry: false });
  const byDay = useQuery({
    queryKey: ["admin-usage", "day", conn.baseURL],
    queryFn: () => getAdminUsage(conn, { scope: "global", group_by: "day", from: daysAgoISO(14) }),
    retry: false,
  });
  const byModel = useQuery({
    queryKey: ["admin-usage", "model", conn.baseURL],
    queryFn: () => getAdminUsage(conn, { scope: "global", group_by: "model", from: daysAgoISO(14) }),
    retry: false,
  });

  const breakers = ready.data?.reliability ?? [];
  const totals = byDay.data?.totals;
  const rows = byDay.data?.rows ?? [];
  const hasUsage = !byDay.isError && !!totals;
  const errRate = totals && totals.requests > 0 ? totals.errors / totals.requests : 0;
  const cacheRate = totals && totals.input_tokens > 0 ? totals.cached_input_tokens / totals.input_tokens : 0;
  const costPer1k = totals && totals.total_tokens > 0 ? (totals.cost_usd / totals.total_tokens) * 1000 : 0;

  const modelIds = (models.data ?? []).map((m) => m.id);
  const providers = providersFromModels(modelIds);

  const at = (fn: (r: AdminUsageRow) => number, color: string, name: string): Series => ({
    name,
    color,
    points: rows.map((r) => ({ x: Date.parse(r.key), y: fn(r) })),
  });
  const reqSeries = [at((r) => r.requests, "var(--series-1)", "requests")];
  const costSeries = [at((r) => r.cost_usd, "var(--series-3)", "cost")];
  const latSeries = [at((r) => r.avg_total_latency_ms, "var(--series-5)", "latency")];

  const modelRows = (byModel.data?.rows ?? []).slice().sort((a, b) => b.cost_usd - a.cost_usd);
  const topModels = modelRows.slice(0, 6).map((r) => ({ label: r.key, value: r.cost_usd }));

  useReveal(scope, [hasUsage, breakers.length, advanced], !advanced);

  return (
    <div ref={scope}>
      <PageHeader title="Overview" />

      {!hasUsage && (
        <Panel reveal>
          <div className="prose">
            Usage analytics come from the admin endpoints (<code>/v1/admin/usage</code>). They ship with this
            console&rsquo;s gateway build and need <code>control_plane.enabled</code>. Everything else works without them.
          </div>
        </Panel>
      )}

      {hasUsage && totals && (advanced ? (
        /* ---------- ADVANCED: information-first, raw + complete, static ---------- */
        <div className="grid">
          <div className="col-12">
            <ReadoutGrid>
              <Readout label="Requests" value={fmtFull(totals.requests)} />
              <Readout label="Errors" value={fmtFull(totals.errors)} tone={totals.errors > 0 ? "var(--status-critical)" : undefined} />
              <Readout label="Error rate" value={fmtPct(errRate)} tone={errRate >= 0.05 ? "var(--status-critical)" : undefined} />
              <Readout label="Tokens total" value={fmtFull(totals.total_tokens)} />
              <Readout label="Input" value={fmtFull(totals.input_tokens)} />
              <Readout label="Output" value={fmtFull(totals.output_tokens)} />
              <Readout label="Cached in" value={fmtFull(totals.cached_input_tokens)} />
              <Readout label="Cache hit" value={fmtPct(cacheRate)} />
              <Readout label="Est. cost" value={fmtUSD(totals.cost_usd)} />
              <Readout label="Cost / 1k tok" value={fmtUSD(costPer1k)} />
              <Readout label="Avg latency" value={fmtMs(totals.avg_total_latency_ms)} />
              <Readout label="Providers" value={`${providers.length}`} sub={`${models.data?.length ?? 0} models`} />
            </ReadoutGrid>
          </div>

          <Panel className="col-6" title="Requests / day" reveal>
            <TimeSeries series={reqSeries} height={150} />
          </Panel>
          <Panel className="col-6" title="Est. cost / day" reveal>
            <TimeSeries series={costSeries} height={150} format={fmtUSD} />
          </Panel>
          <Panel className="col-6" title="Avg latency / day" reveal>
            <TimeSeries series={latSeries} height={150} format={fmtMs} />
          </Panel>
          <Panel className="col-6" title="By model" reveal>
            <ModelTable rows={modelRows} />
          </Panel>

          <Panel className="col-12" title="Provider health" reveal>
            <SystemStatus advanced ready={ready.data} breakers={breakers} providers={providers} models={models.data?.length ?? 0} />
          </Panel>
        </div>
      ) : (
        /* ---------- STANDARD: experience-first, curated, animated, airy ---------- */
        <div className="stack-lg">
          <div className="grid">
            <div className="col-3"><StatTile label="Requests · 14d" value={totals.requests} spark={rows.map((r) => r.requests)} /></div>
            <div className="col-3"><StatTile label="Tokens · 14d" value={totals.total_tokens} format={fmtInt} /></div>
            <div className="col-3"><StatTile label="Est. cost · 14d" value={totals.cost_usd} format={fmtUSD} spark={rows.map((r) => r.cost_usd)} /></div>
            <div className="col-3"><StatTile label="Error rate" value={errRate} format={fmtPct} tone={errRate >= 0.05 ? "var(--status-critical)" : undefined} /></div>
          </div>

          <div className="grid">
            <Panel className="col-8" title="Requests · last 14 days" reveal>
              <TimeSeries series={reqSeries} />
            </Panel>
            <Panel className="col-4" title="Top models by cost" reveal>
              <BarList items={topModels} format={fmtUSD} />
            </Panel>
            <Panel className="col-12" title="System health" reveal>
              <SystemStatus ready={ready.data} breakers={breakers} providers={providers} models={models.data?.length ?? 0} />
            </Panel>
          </div>
        </div>
      ))}
    </div>
  );
}

function ModelTable({ rows }: { rows: AdminUsageRow[] }) {
  const cols: Column<AdminUsageRow>[] = [
    { key: "key", header: "model", render: (r) => <span className="num" style={{ fontSize: 12 }}>{r.key}</span> },
    { key: "req", header: "Req", align: "right", mono: true, render: (r) => fmtInt(r.requests) },
    { key: "tok", header: "Tokens", align: "right", mono: true, render: (r) => fmtInt(r.total_tokens) },
    { key: "cost", header: "Cost", align: "right", mono: true, render: (r) => fmtUSD(r.cost_usd) },
    { key: "err", header: "Err", align: "right", mono: true, render: (r) => (r.errors > 0 ? <span style={{ color: "var(--status-critical)" }}>{fmtInt(r.errors)}</span> : "0") },
  ];
  return <DataTable columns={cols} rows={rows} rowKey={(r) => r.key} empty="No model usage in range." />;
}

/** Store/cache/provider status, shared by both modes (rendered denser in advanced). */
function SystemStatus({
  advanced,
  ready,
  breakers,
  providers,
  models,
}: {
  advanced?: boolean;
  ready: { store?: string; cache?: string } | undefined;
  breakers: { provider: string; breaker: "closed" | "half_open" | "open"; health_score: number; error_rate: number; ewma_latency_ms: number }[];
  providers: { name: string; models: number }[];
  models: number;
}) {
  const ok = (s: string | undefined) => (s === "ok" ? "var(--status-good)" : "var(--status-warning)");
  return (
    <div className="stack-sm">
      <div style={{ display: "flex", flexWrap: "wrap", gap: advanced ? 20 : 28, alignItems: "center" }}>
        <Dot color={ok(ready?.store)} label={`store ${ready?.store ?? "unknown"}`} />
        <Dot color={ok(ready?.cache)} label={`cache ${ready?.cache ?? "unknown"}`} />
        <span className="meta" style={{ textTransform: "none" }}>
          {providers.length} providers · {models} models
        </span>
      </div>

      {breakers.length > 0 ? (
        <div className="scroll-x">
          <div style={{ display: "flex", gap: advanced ? 1 : 10, background: advanced ? "var(--border)" : undefined, minWidth: "min-content" }}>
            {breakers.map((h) => (
              <div key={h.provider} style={{ background: "var(--surface-1)", border: advanced ? "none" : "1px solid var(--border)", borderRadius: "var(--radius-1)", padding: "10px 12px", minWidth: 150 }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 10, marginBottom: 6 }}>
                  <span style={{ fontSize: 13, fontWeight: 600 }}>{h.provider}</span>
                  <StatusChip state={h.breaker} />
                </div>
                <div className="meta">health {fmtPct(h.health_score)} · err {fmtPct(h.error_rate)} · {fmtMs(h.ewma_latency_ms)}</div>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <div className="scroll-x">
          <div style={{ display: "flex", gap: advanced ? 1 : 10, background: advanced ? "var(--border)" : undefined, minWidth: "min-content" }}>
            {providers.map((p) => (
              <div key={p.name} style={{ background: "var(--surface-1)", border: advanced ? "none" : "1px solid var(--border)", borderRadius: "var(--radius-1)", padding: "10px 12px", minWidth: 140 }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 10, marginBottom: 6 }}>
                  <span style={{ fontSize: 13, fontWeight: 600 }}>{p.name}</span>
                  <span className="num" style={{ fontSize: 12, color: "var(--text-2)" }}>{p.models}</span>
                </div>
                <div className="meta">idle · no live traffic</div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function Dot({ color, label }: { color: string; label: string }) {
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 7 }}>
      <span aria-hidden style={{ width: 8, height: 8, borderRadius: "50%", background: color }} />
      <span className="meta" style={{ textTransform: "none" }}>{label}</span>
    </span>
  );
}
