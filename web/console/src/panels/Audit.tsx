import { useCallback, useEffect, useRef, useState } from "react";
import { listAuditEvents } from "../api/resources";
import { useConn } from "../state/connection";
import { ErrorNote, PageHeader, Panel, Spinner, Tag } from "../components/ui";
import { DataTable, type Column } from "../components/DataTable";
import { FilterRow } from "../components/Filters";
import type { AuditEvent } from "../api/types";
import { useReveal } from "../motion/hooks";

export function Audit() {
  const conn = useConn();
  const scope = useRef<HTMLDivElement>(null);
  const [kind, setKind] = useState("");
  const [items, setItems] = useState<AuditEvent[]>([]);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [done, setDone] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(
    async (reset: boolean) => {
      setLoading(true);
      setError(null);
      try {
        const res = await listAuditEvents(conn, {
          kind: kind || undefined,
          cursor: reset ? undefined : cursor,
          limit: 50,
        });
        const data = res.data ?? [];
        setItems((prev) => (reset ? data : [...prev, ...data]));
        setCursor(res.next_cursor);
        setDone(data.length < 50);
      } catch (e) {
        setError((e as Error).message);
      } finally {
        setLoading(false);
      }
    },
    [conn, kind, cursor]
  );

  // Reset and reload when the filter changes.
  useEffect(() => {
    setItems([]);
    setCursor(undefined);
    setDone(false);
    void load(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kind]);

  useReveal(scope, [items.length]);

  const cols: Column<AuditEvent>[] = [
    { key: "time", header: "When", mono: true, render: (e) => new Date(e.created_at).toLocaleString() },
    { key: "kind", header: "Event", render: (e) => <Tag tone="accent">{e.kind}</Tag> },
    { key: "resource", header: "Resource", render: (e) => (
        <span className="num" style={{ fontSize: 12 }}>{e.resource_type}{e.resource_id ? ` · ${e.resource_id}` : ""}</span>
      ) },
    { key: "actor", header: "Actor", mono: true, render: (e) => <span style={{ fontSize: 12, color: "var(--text-3)" }}>{e.actor_key_id || "—"}</span> },
    { key: "meta", header: "Metadata", render: (e) => (
        <details>
          <summary className="meta" style={{ cursor: "pointer", textTransform: "none" }}>view</summary>
          <pre className="mono" style={{ fontSize: 11, whiteSpace: "pre-wrap", margin: "6px 0 0", color: "var(--text-2)" }}>{e.metadata_json || "{}"}</pre>
        </details>
      ) },
  ];

  return (
    <div ref={scope}>
      <PageHeader title="Audit Log" />

      <FilterRow>
        <input className="input" placeholder="Filter by kind (e.g. virtual_key.created)" value={kind} onChange={(e) => setKind(e.target.value)} style={{ maxWidth: 300 }} />
      </FilterRow>

      {error && <ErrorNote message={error} />}
      <Panel>
        <DataTable columns={cols} rows={items} rowKey={(e) => e.id} empty="No audit events." />
        {loading && <Spinner />}
        {!done && !loading && items.length > 0 && (
          <div style={{ textAlign: "center", marginTop: 14 }}>
            <button className="btn btn-sm" onClick={() => void load(false)}>
              Load more
            </button>
          </div>
        )}
      </Panel>
    </div>
  );
}
