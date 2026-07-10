import type { ReactNode } from "react";
import type { BreakerState } from "../api/types";

/**
 * Page header: a title plus an optional right-aligned actions slot. Standard renders
 * it airy and editorial; Advanced renders it as a dense telemetry bar with a hairline
 * rule (styled in base.css by data-mode).
 *
 * Deliberately no subtitle. The nav label already names the page and the content shows
 * what it holds, so a descriptive sub-line only restates the visible — the classic
 * "AI-generated" narration tell. Non-obvious semantics belong at their point of use
 * (a form hint, a column header), not under every heading.
 */
export function PageHeader({ title, actions }: { title: ReactNode; actions?: ReactNode }) {
  return (
    <header className="page-head">
      <div className="page-head-main">
        <h1 className="page-title">{title}</h1>
      </div>
      {actions ? <div className="page-head-actions">{actions}</div> : null}
    </header>
  );
}

/**
 * A single dense readout: label over a mono value. Advanced-mode KPI strips are grids
 * of these (hairline-separated, no card chrome) so a technical reader scans many exact
 * figures at once — the information-first counterpart to Standard's animated StatTile.
 */
export function Readout({
  label,
  value,
  sub,
  tone,
}: {
  label: ReactNode;
  value: ReactNode;
  sub?: ReactNode;
  tone?: string;
}) {
  return (
    <div className="readout">
      <div className="readout-label">{label}</div>
      <output className="readout-value num" style={tone ? { color: tone } : undefined}>
        {value}
      </output>
      {sub ? <div className="readout-sub">{sub}</div> : null}
    </div>
  );
}

/** Hairline grid wrapper for Readouts. Columns are fixed-responsive (2/3/4/6) via
 *  container queries in base.css, so a 12-readout strip always fills its rows. */
export function ReadoutGrid({ children }: { children: ReactNode }) {
  return <div className="readout-grid">{children}</div>;
}

export function Panel({
  title,
  actions,
  children,
  className,
  reveal,
}: {
  title?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  reveal?: boolean;
}) {
  return (
    <section className={`panel ${className ?? ""}`} {...(reveal ? { "data-reveal": "" } : {})}>
      {title !== undefined && (
        <header className="panel-head">
          <h2 className="panel-title">{title}</h2>
          {actions}
        </header>
      )}
      <div className="panel-body">{children}</div>
    </section>
  );
}

export function EmptyState({ title, hint }: { title: string; hint?: ReactNode }) {
  return (
    <div style={{ textAlign: "center", padding: "40px 16px", color: "var(--text-3)" }}>
      <div style={{ fontSize: 14, color: "var(--text-2)", marginBottom: 6 }}>{title}</div>
      {hint && <div style={{ fontSize: 12, maxWidth: 420, margin: "0 auto" }}>{hint}</div>}
    </div>
  );
}

export function Spinner({ label }: { label?: string }) {
  return (
    <div className="meta" role="status" style={{ padding: "16px 0" }}>
      {label ?? "Loading…"}
    </div>
  );
}

export function ErrorNote({ message }: { message: string }) {
  return (
    <div
      role="alert"
      style={{
        padding: "10px 12px",
        border: "1px solid var(--danger)",
        borderRadius: "var(--radius-1)",
        background: "var(--danger-weak)",
        color: "var(--danger)",
        fontSize: 13,
      }}
    >
      {message}
    </div>
  );
}

export function Tag({ children, tone }: { children: ReactNode; tone?: "accent" | "muted" }) {
  const style =
    tone === "accent"
      ? { background: "var(--accent-weak)", color: "var(--accent)" }
      : undefined;
  return (
    <span className="tag" style={style}>
      {children}
    </span>
  );
}

const BREAKER_STATUS: Record<BreakerState, { color: string; label: string }> = {
  closed: { color: "var(--status-good)", label: "closed" },
  half_open: { color: "var(--status-warning)", label: "half-open" },
  open: { color: "var(--status-critical)", label: "open" },
};

/** Status pill with a filled dot + text label — never color alone (dataviz rule). */
export function StatusChip({ state }: { state: BreakerState }) {
  const s = BREAKER_STATUS[state];
  return (
    <span className="tag" style={{ background: "transparent", border: "1px solid var(--border)" }}>
      <span
        aria-hidden
        style={{ width: 8, height: 8, borderRadius: "50%", background: s.color, display: "inline-block" }}
      />
      <span style={{ color: "var(--text-1)" }}>{s.label}</span>
    </span>
  );
}

export function Field({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div className="field" style={{ marginBottom: 14 }}>
      <label>{label}</label>
      {children}
    </div>
  );
}

export function KeyVal({ k, children }: { k: ReactNode; children: ReactNode }) {
  return (
    <div style={{ display: "flex", justifyContent: "space-between", gap: 16, padding: "6px 0" }}>
      <span className="meta">{k}</span>
      <span className="num" style={{ fontSize: 13 }}>
        {children}
      </span>
    </div>
  );
}
