import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listModels } from "../api/resources";
import { useConn } from "../state/connection";
import { ErrorNote, PageHeader, Panel, Spinner, Tag } from "../components/ui";
import { DataTable, type Column } from "../components/DataTable";
import type { Model } from "../api/types";

export function Models() {
  const conn = useConn();
  const [search, setSearch] = useState("");
  const [modality, setModality] = useState("");
  const q = useQuery({ queryKey: ["models", conn.baseURL], queryFn: () => listModels(conn) });

  const models = q.data ?? [];
  const modalities = useMemo(
    () => Array.from(new Set(models.map((m) => m.modality).filter(Boolean))).sort() as string[],
    [models]
  );
  const filtered = models.filter((m) => {
    if (modality && m.modality !== modality) return false;
    if (search) {
      const s = search.toLowerCase();
      return (
        m.id.toLowerCase().includes(s) ||
        (m.display_name ?? "").toLowerCase().includes(s) ||
        (m.family_id ?? "").toLowerCase().includes(s)
      );
    }
    return true;
  });

  const cols: Column<Model>[] = [
    {
      key: "id",
      header: "Model",
      render: (m) => (
        <div>
          <div className="num" style={{ fontSize: 12 }}>{m.id}</div>
          {m.display_name && <div style={{ fontSize: 12, color: "var(--text-3)" }}>{m.display_name}</div>}
        </div>
      ),
    },
    { key: "modality", header: "Modality", render: (m) => <Tag>{m.modality ?? "—"}</Tag> },
    {
      key: "caps",
      header: "Capabilities",
      render: (m) => (
        <div style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
          {(m.capabilities ?? []).slice(0, 5).map((c) => (
            <Tag key={c}>{c}</Tag>
          ))}
        </div>
      ),
    },
    { key: "status", header: "Status", render: (m) => <span className="meta">{m.status ?? "—"}</span> },
    { key: "verif", header: "Verification", render: (m) => <span className="meta">{m.verification_class ?? "—"}</span> },
    {
      key: "tier",
      header: "Cost / Latency",
      render: (m) => (
        <span className="meta">
          {m.cost_tier ?? "—"} / {m.latency_tier ?? "—"}
        </span>
      ),
    },
    {
      key: "resolves",
      header: "Resolves to",
      render: (m) => (m.resolves_to ? <span className="num" style={{ fontSize: 12 }}>{m.resolves_to}</span> : <span className="meta">exact</span>),
    },
  ];

  return (
    <div>
      <PageHeader title="Models" />

      <div style={{ display: "flex", gap: 10, marginBottom: 14, flexWrap: "wrap" }}>
        <input className="input" placeholder="Search id, name, family…" value={search} onChange={(e) => setSearch(e.target.value)} style={{ maxWidth: 280 }} />
        <select className="select" value={modality} onChange={(e) => setModality(e.target.value)} style={{ maxWidth: 180 }}>
          <option value="">All modalities</option>
          {modalities.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </select>
      </div>

      {q.isLoading && <Spinner />}
      {q.isError && <ErrorNote message={(q.error as Error).message} />}
      {q.data && (
        <Panel>
          <DataTable columns={cols} rows={filtered} rowKey={(m) => m.id} empty="No models match the filter." />
        </Panel>
      )}
    </div>
  );
}
