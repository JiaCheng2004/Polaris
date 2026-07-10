import { useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getAdminUsage, listProjects } from "../api/resources";
import { useConn, useConnection } from "../state/connection";
import { ErrorNote, PageHeader, Panel, Readout, ReadoutGrid, Spinner } from "../components/ui";
import { useTheme } from "../state/theme";
import { DataTable, type Column } from "../components/DataTable";
import { FilterRow, PresetRange, rangeForDays } from "../components/Filters";
import { BarList, StatTile, TimeSeries, fmtInt, fmtMs, fmtPct, fmtUSD, type Series } from "../viz/charts";
import type { AdminUsageRow, UsageGroupBy, UsageScope } from "../api/types";
import { useReveal } from "../motion/hooks";

const GROUPS: UsageGroupBy[] = ["day", "model", "provider", "modality", "status", "token_source", "cost_source"];

function TokenComposition({ t }: { t: AdminUsageRow }) {
  const freshInput = Math.max(0, t.input_tokens - t.cached_input_tokens);
  const segs = [
    { label: "fresh input", value: freshInput, color: "var(--series-1)" },
    { label: "cached input", value: t.cached_input_tokens, color: "var(--series-2)" },
    { label: "cache write", value: t.cache_write_tokens, color: "var(--series-3)" },
    { label: "output", value: t.output_tokens, color: "var(--series-5)" },
  ].filter((s) => s.value > 0);
  const total = segs.reduce((a, s) => a + s.value, 0) || 1;
  return (
    <div>
      <div style={{ display: "flex", height: 12, borderRadius: 3, overflow: "hidden", gap: 2, background: "var(--surface-inset)" }}>
        {segs.map((s) => (
          <div key={s.label} title={`${s.label}: ${fmtInt(s.value)}`} style={{ width: `${(s.value / total) * 100}%`, background: s.color }} />
        ))}
      </div>
      <div style={{ display: "flex", gap: 14, flexWrap: "wrap", marginTop: 8 }}>
        {segs.map((s) => (
          <span key={s.label} className="meta" style={{ display: "inline-flex", alignItems: "center", gap: 5, textTransform: "none" }}>
            <span style={{ width: 8, height: 8, background: s.color, borderRadius: 2 }} /> {s.label} {fmtInt(s.value)}
          </span>
        ))}
      </div>
    </div>
  );
}

const fmtFull = (n: number) => Intl.NumberFormat("en").format(Math.round(n));

export function Usage() {
  const conn = useConn();
  const { caps } = useConnection();
  const { mode } = useTheme();
  const advanced = mode === "advanced";
  const scopeRef = useRef<HTMLDivElement>(null);
  const [days, setDays] = useState(30);
  const [groupBy, setGroupBy] = useState<UsageGroupBy>("day");
  const [scope, setScope] = useState<UsageScope>("global");
  const [projectId, setProjectId] = useState("");

  const range = useMemo(() => rangeForDays(days), [days]);
  const projects = useQuery({
    queryKey: ["projects", conn.baseURL],
    queryFn: () => listProjects(conn),
    enabled: caps.controlPlaneEnabled && scope === "project",
  });

  const q = useQuery({
    queryKey: ["admin-usage", conn.baseURL, groupBy, scope, projectId, range.from, range.to],
    queryFn: () =>
      getAdminUsage(conn, {
        scope,
        group_by: groupBy,
        project_id: scope === "project" ? projectId : undefined,
        from: range.from,
        to: range.to,
      }),
    enabled: scope !== "project" || !!projectId,
    retry: false,
  });

  const report = q.data;
  const totals = report?.totals;
  const errRate = totals && totals.requests > 0 ? totals.errors / totals.requests : 0;
  const cacheRate = totals && totals.input_tokens > 0 ? totals.cached_input_tokens / totals.input_tokens : 0;
  const costPer1k = totals && totals.total_tokens > 0 ? (totals.cost_usd / totals.total_tokens) * 1000 : 0;

  const series: Series[] =
    groupBy === "day" && report
      ? [
          { name: "requests", color: "var(--series-1)", points: report.rows.map((r) => ({ x: Date.parse(r.key), y: r.requests })) },
        ]
      : [];
  const topByCost = (report?.rows ?? [])
    .slice()
    .sort((a, b) => b.cost_usd - a.cost_usd)
    .slice(0, 8)
    .map((r) => ({ label: r.key, value: r.cost_usd }));

  useReveal(scopeRef, [groupBy, scope, days, !!report, advanced], !advanced);

  const cols: Column<AdminUsageRow>[] = [
    { key: "key", header: groupBy, render: (r) => <span className="num" style={{ fontSize: 12 }}>{r.key}</span> },
    { key: "requests", header: "Requests", align: "right", mono: true, render: (r) => fmtInt(r.requests) },
    { key: "tokens", header: "Tokens", align: "right", mono: true, render: (r) => fmtInt(r.total_tokens) },
    { key: "cached", header: "Cached", align: "right", mono: true, render: (r) => fmtInt(r.cached_input_tokens) },
    { key: "cost", header: "Cost", align: "right", mono: true, render: (r) => fmtUSD(r.cost_usd) },
    { key: "errors", header: "Errors", align: "right", mono: true, render: (r) => (r.errors > 0 ? <span style={{ color: "var(--status-critical)" }}>{fmtInt(r.errors)}</span> : "0") },
    { key: "lat", header: "Avg latency", align: "right", mono: true, render: (r) => fmtMs(r.avg_total_latency_ms) },
  ];

  return (
    <div ref={scopeRef}>
      <PageHeader title="Usage & Cost" />

      <FilterRow>
        <PresetRange valueDays={days} onChange={setDays} />
        <select className="select" value={groupBy} onChange={(e) => setGroupBy(e.target.value as UsageGroupBy)} style={{ width: 160 }}>
          {GROUPS.map((g) => (
            <option key={g} value={g}>
              by {g}
            </option>
          ))}
        </select>
        <select className="select" value={scope} onChange={(e) => setScope(e.target.value as UsageScope)} style={{ width: 130 }}>
          <option value="global">global</option>
          {caps.controlPlaneEnabled && <option value="project">project</option>}
        </select>
        {scope === "project" && (
          <select className="select" value={projectId} onChange={(e) => setProjectId(e.target.value)} style={{ width: 220 }}>
            <option value="">Select project…</option>
            {(projects.data ?? []).map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        )}
      </FilterRow>

      {q.isLoading && <Spinner />}
      {q.isError && <ErrorNote message={`${(q.error as Error).message} — usage analytics require the admin endpoints and control_plane.enabled.`} />}

      {report && totals && (
        <>
          {advanced ? (
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
              <Readout label="Window" value={`${days}d`} sub={scope} />
            </ReadoutGrid>
          ) : (
            <div className="grid">
              <div className="col-3"><StatTile label="Requests" value={totals.requests} /></div>
              <div className="col-3"><StatTile label="Total tokens" value={totals.total_tokens} format={fmtInt} /></div>
              <div className="col-3"><StatTile label="Est. cost" value={totals.cost_usd} format={fmtUSD} /></div>
              <div className="col-3"><StatTile label="Error rate" value={errRate} format={fmtPct} tone={errRate >= 0.05 ? "var(--status-critical)" : undefined} /></div>
            </div>
          )}

          <div className="grid" style={{ marginTop: "var(--grid-gap)" }}>
            {groupBy === "day" ? (
              <>
                <Panel className="col-8" title="Requests over time" reveal>
                  <TimeSeries series={series} />
                </Panel>
                <Panel className="col-4" title={`Top ${groupBy} by cost`} reveal>
                  <BarList items={topByCost} format={fmtUSD} />
                </Panel>
              </>
            ) : (
              <Panel className="col-12" title={`Top ${groupBy} by cost`} reveal>
                <BarList items={topByCost} format={fmtUSD} />
              </Panel>
            )}

            <Panel className="col-12" title="Token economics" reveal actions={<span className="meta">cache hit {fmtPct(cacheRate)}</span>}>
              <TokenComposition t={totals} />
            </Panel>

            <Panel className="col-12" title={`Breakdown by ${groupBy}`} reveal>
              <DataTable columns={cols} rows={report.rows} rowKey={(r) => r.key} empty="No usage in this range." />
            </Panel>
          </div>
        </>
      )}
    </div>
  );
}
