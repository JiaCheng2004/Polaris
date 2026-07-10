import { useRef, useState, type ReactNode } from "react";
import { useCountUp } from "../motion/hooks";
import { useTheme } from "../state/theme";

// Formatters (tabular, compact).
export const fmtInt = (n: number) =>
  Math.abs(n) >= 1000 ? Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(n) : String(Math.round(n));
export const fmtUSD = (n: number) =>
  "$" + Intl.NumberFormat("en", { maximumFractionDigits: n < 10 ? 4 : 2 }).format(n);
export const fmtPct = (n: number) => (n * 100).toFixed(n < 0.1 ? 2 : 1) + "%";
export const fmtMs = (n: number) => (n >= 1000 ? (n / 1000).toFixed(2) + "s" : Math.round(n) + "ms");

/* ---------- Stat tile (value + delta + optional sparkline) ---------- */
export function StatTile({
  label,
  value,
  format = fmtInt,
  spark,
  tone,
}: {
  label: string;
  value: number;
  format?: (n: number) => string;
  spark?: number[];
  tone?: string;
}) {
  const { mode } = useTheme();
  const ref = useCountUp(value, format, mode === "standard");
  return (
    <div
      data-reveal=""
      style={{
        background: "var(--surface-1)",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius-2)",
        padding: "14px 16px",
        display: "flex",
        flexDirection: "column",
        gap: 6,
        minWidth: 0,
      }}
    >
      <div className="meta">{label}</div>
      <div
        className="num"
        style={{ fontSize: "clamp(22px, 3.2cqw, 30px)", fontWeight: 700, color: tone ?? "var(--text-1)", lineHeight: 1.1 }}
      >
        <span ref={ref}>{format(value)}</span>
      </div>
      {spark && spark.length > 1 && <Sparkline data={spark} />}
    </div>
  );
}

/* ---------- Sparkline ---------- */
export function Sparkline({ data, height = 26 }: { data: number[]; height?: number }) {
  const w = 120;
  const max = Math.max(...data, 1);
  const min = Math.min(...data, 0);
  const span = max - min || 1;
  const pts = data
    .map((d, i) => `${(i / (data.length - 1)) * w},${height - ((d - min) / span) * (height - 2) - 1}`)
    .join(" ");
  return (
    <svg width="100%" height={height} viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" aria-hidden>
      <polyline points={pts} fill="none" stroke="var(--series-1)" strokeWidth={1.5} vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

/* ---------- Meter (track + fill, thresholded) ---------- */
export function Meter({
  value,
  max,
  label,
  valueText,
  tone,
}: {
  value: number;
  max: number;
  label?: ReactNode;
  valueText?: string;
  tone?: string;
}) {
  const pct = max > 0 ? Math.min(1, value / max) : 0;
  const color = tone ?? (pct >= 1 ? "var(--status-critical)" : pct >= 0.8 ? "var(--status-warning)" : "var(--series-1)");
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
      {(label || valueText) && (
        <div style={{ display: "flex", justifyContent: "space-between", fontSize: 12 }}>
          <span className="meta">{label}</span>
          <span className="num" style={{ color: "var(--text-1)" }}>
            {valueText}
          </span>
        </div>
      )}
      <div style={{ height: 6, background: "var(--surface-inset)", borderRadius: 3, overflow: "hidden" }}>
        <div style={{ width: `${pct * 100}%`, height: "100%", background: color, transition: "width .4s var(--ease)" }} />
      </div>
    </div>
  );
}

/* ---------- Horizontal bar list (top-N by magnitude, sequential) ---------- */
export function BarList({
  items,
  format = fmtInt,
}: {
  items: { label: string; value: number }[];
  format?: (n: number) => string;
}) {
  const max = Math.max(...items.map((i) => i.value), 1);
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      {items.map((it) => (
        <div key={it.label} style={{ display: "grid", gridTemplateColumns: "minmax(90px, 34%) 1fr auto", gap: 10, alignItems: "center" }}>
          <span style={{ fontSize: 12, color: "var(--text-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={it.label}>
            {it.label}
          </span>
          <div style={{ height: 8, background: "var(--surface-inset)", borderRadius: 2 }}>
            <div style={{ width: `${(it.value / max) * 100}%`, height: "100%", background: "var(--series-1)", borderRadius: 2, transition: "width .4s var(--ease)" }} />
          </div>
          <span className="num" style={{ fontSize: 12, color: "var(--text-1)" }}>
            {format(it.value)}
          </span>
        </div>
      ))}
    </div>
  );
}

/* ---------- Time series (single or multi line, hover crosshair) ---------- */
export interface Series {
  name: string;
  color: string;
  points: { x: number; y: number }[]; // x = epoch ms
}
export function TimeSeries({
  series,
  height = 180,
  format = fmtInt,
}: {
  series: Series[];
  height?: number;
  format?: (n: number) => string;
}) {
  const ref = useRef<SVGSVGElement>(null);
  const [hover, setHover] = useState<number | null>(null);
  const W = 640;
  const H = height;
  const padL = 44;
  const padB = 22;
  const padT = 8;
  const padR = 8;

  const allX = series.flatMap((s) => s.points.map((p) => p.x));
  const allY = series.flatMap((s) => s.points.map((p) => p.y));
  if (allX.length === 0) return <div className="meta" style={{ padding: 24 }}>No data in range.</div>;
  const minX = Math.min(...allX);
  const maxX = Math.max(...allX) || minX + 1;
  const maxY = Math.max(...allY, 1);
  const sx = (x: number) => padL + ((x - minX) / (maxX - minX || 1)) * (W - padL - padR);
  const sy = (y: number) => H - padB - (y / maxY) * (H - padT - padB);

  const xs = series[0]?.points.map((p) => p.x) ?? [];
  const onMove = (e: React.PointerEvent) => {
    const svg = ref.current;
    if (!svg || xs.length === 0) return;
    const rect = svg.getBoundingClientRect();
    const px = ((e.clientX - rect.left) / rect.width) * W;
    let nearest = 0;
    let best = Infinity;
    xs.forEach((x, i) => {
      const d = Math.abs(sx(x) - px);
      if (d < best) { best = d; nearest = i; }
    });
    setHover(nearest);
  };

  const ticks = [0, 0.5, 1].map((f) => Math.round(maxY * f));

  return (
    <div>
      <svg
        ref={ref}
        viewBox={`0 0 ${W} ${H}`}
        width="100%"
        height={H}
        role="img"
        onPointerMove={onMove}
        onPointerLeave={() => setHover(null)}
        style={{ display: "block", touchAction: "none" }}
      >
        {ticks.map((t, i) => (
          <g key={i}>
            <line x1={padL} x2={W - padR} y1={sy(t)} y2={sy(t)} stroke="var(--viz-grid)" strokeWidth={1} />
            <text x={padL - 6} y={sy(t) + 3} textAnchor="end" fontSize={9} fill="var(--viz-ink-2)" className="num">
              {fmtInt(t)}
            </text>
          </g>
        ))}
        {series.map((s) => (
          <polyline
            key={s.name}
            fill="none"
            stroke={s.color}
            strokeWidth={2}
            vectorEffect="non-scaling-stroke"
            points={s.points.map((p) => `${sx(p.x)},${sy(p.y)}`).join(" ")}
          />
        ))}
        {hover !== null && xs[hover] !== undefined && (
          <line x1={sx(xs[hover])} x2={sx(xs[hover])} y1={padT} y2={H - padB} stroke="var(--border-strong)" strokeWidth={1} />
        )}
        {hover !== null &&
          series.map((s) => {
            const p = s.points[hover];
            if (!p) return null;
            return <circle key={s.name} cx={sx(p.x)} cy={sy(p.y)} r={3} fill={s.color} stroke="var(--viz-surface)" strokeWidth={1.5} />;
          })}
      </svg>
      {hover !== null && xs[hover] !== undefined && (
        <div className="mono" style={{ fontSize: 11, color: "var(--text-2)", marginTop: 4 }}>
          {new Date(xs[hover]).toISOString().slice(0, 10)} —{" "}
          {series.map((s) => (
            <span key={s.name} style={{ marginRight: 10 }}>
              <span style={{ color: s.color }}>■</span> {s.name} {format(s.points[hover]?.y ?? 0)}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
