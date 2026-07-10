export interface Range {
  from: string;
  to: string;
  label: string;
}

const PRESETS: { days: number; label: string }[] = [
  { days: 1, label: "24h" },
  { days: 7, label: "7d" },
  { days: 30, label: "30d" },
  { days: 90, label: "90d" },
];

export function rangeForDays(days: number): Range {
  const to = new Date();
  const from = new Date(to.getTime() - days * 86400_000);
  return { from: from.toISOString(), to: to.toISOString(), label: `${days}d` };
}

export function PresetRange({ valueDays, onChange }: { valueDays: number; onChange: (days: number) => void }) {
  return (
    <div style={{ display: "inline-flex", border: "1px solid var(--border-strong)", borderRadius: "var(--radius-1)", overflow: "hidden" }}>
      {PRESETS.map((p) => (
        <button
          key={p.days}
          onClick={() => onChange(p.days)}
          className="num"
          style={{
            padding: "6px 12px",
            fontSize: 12,
            border: "none",
            borderRight: "1px solid var(--border)",
            background: valueDays === p.days ? "var(--accent-weak)" : "var(--surface-1)",
            color: valueDays === p.days ? "var(--accent)" : "var(--text-2)",
            cursor: "pointer",
          }}
        >
          {p.label}
        </button>
      ))}
    </div>
  );
}

export function FilterRow({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap", marginBottom: 16 }}>{children}</div>
  );
}
