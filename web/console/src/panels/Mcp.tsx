import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createMCPBinding, listMCPBindings, listToolsets } from "../api/resources";
import { useConn } from "../state/connection";
import { useToast } from "../components/Toast";
import { ErrorNote, Field, PageHeader, Panel, Spinner, Tag } from "../components/ui";
import { DataTable, type Column } from "../components/DataTable";
import { Drawer } from "../components/Modal";
import type { MCPBinding, MCPBindingKind } from "../api/types";

function headerCount(json: string): number {
  try {
    const o = JSON.parse(json || "{}");
    return Object.keys(o).length;
  } catch {
    return 0;
  }
}

export function Mcp() {
  const conn = useConn();
  const qc = useQueryClient();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [f, setF] = useState<{ name: string; kind: MCPBindingKind; upstream_url: string; toolset_id: string; headers: string }>({
    name: "", kind: "upstream_proxy", upstream_url: "", toolset_id: "", headers: "",
  });
  const [headerErr, setHeaderErr] = useState<string | null>(null);

  const toolsets = useQuery({ queryKey: ["toolsets", conn.baseURL], queryFn: () => listToolsets(conn) });
  const q = useQuery({ queryKey: ["mcp", conn.baseURL], queryFn: () => listMCPBindings(conn) });

  const create = useMutation({
    mutationFn: () => {
      let headers: Record<string, string> | undefined;
      if (f.kind === "upstream_proxy" && f.headers.trim()) {
        headers = JSON.parse(f.headers) as Record<string, string>;
      }
      return createMCPBinding(conn, {
        name: f.name,
        kind: f.kind,
        upstream_url: f.kind === "upstream_proxy" ? f.upstream_url : undefined,
        toolset_id: f.kind === "local_toolset" ? f.toolset_id : undefined,
        headers,
      });
    },
    onSuccess: () => {
      toast.push("Binding created", "success");
      setOpen(false);
      setF({ name: "", kind: "upstream_proxy", upstream_url: "", toolset_id: "", headers: "" });
      void qc.invalidateQueries({ queryKey: ["mcp", conn.baseURL] });
    },
    onError: (e) => toast.push((e as Error).message, "error"),
  });

  const submit = () => {
    setHeaderErr(null);
    if (f.kind === "upstream_proxy" && f.headers.trim()) {
      try {
        JSON.parse(f.headers);
      } catch {
        setHeaderErr("Headers must be a JSON object, e.g. {\"Authorization\":\"Bearer …\"}");
        return;
      }
    }
    create.mutate();
  };

  const cols: Column<MCPBinding>[] = [
    { key: "name", header: "Name", render: (b) => <div>{b.name}<div className="num" style={{ fontSize: 11, color: "var(--text-3)" }}>{b.id}</div></div> },
    { key: "kind", header: "Kind", render: (b) => <Tag tone="accent">{b.kind}</Tag> },
    { key: "target", header: "Target", mono: true, render: (b) => <span style={{ fontSize: 12 }}>{b.kind === "upstream_proxy" ? b.upstream_url : b.toolset_id || "—"}</span> },
    { key: "headers", header: "Headers", render: (b) => <span className="meta">{headerCount(b.headers_json)} set</span> },
    { key: "en", header: "Enabled", render: (b) => (b.enabled ? <span style={{ color: "var(--status-good)" }}>yes</span> : <Tag>no</Tag>) },
  ];

  return (
    <div>
      <PageHeader
        title="MCP Bindings"
        actions={
          <button className="btn btn-primary btn-sm" onClick={() => setOpen(true)}>New binding</button>
        }
      />

      {q.isLoading && <Spinner />}
      {q.isError && <ErrorNote message={(q.error as Error).message} />}
      {q.data && <Panel><DataTable columns={cols} rows={q.data} rowKey={(b) => b.id} empty="No MCP bindings." /></Panel>}

      {open && (
        <Drawer title="New MCP binding" onClose={() => setOpen(false)}
          footer={<><button className="btn btn-sm" onClick={() => setOpen(false)}>Cancel</button><button className="btn btn-primary btn-sm" disabled={!f.name || create.isPending} onClick={submit}>{create.isPending ? "Saving…" : "Create"}</button></>}>
          <Field label="Name"><input className="input" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Kind"><select className="select" value={f.kind} onChange={(e) => setF({ ...f, kind: e.target.value as MCPBindingKind })}><option value="upstream_proxy">upstream_proxy</option><option value="local_toolset">local_toolset</option></select></Field>
          {f.kind === "upstream_proxy" ? (
            <>
              <Field label="Upstream URL"><input className="input mono" value={f.upstream_url} onChange={(e) => setF({ ...f, upstream_url: e.target.value })} placeholder="https://mcp.example.com" /></Field>
              <Field label="Headers (JSON, optional — stored securely, never shown back)"><textarea className="textarea" value={f.headers} onChange={(e) => setF({ ...f, headers: e.target.value })} placeholder='{"Authorization":"Bearer …"}' /></Field>
              {headerErr && <ErrorNote message={headerErr} />}
            </>
          ) : (
            <Field label="Toolset"><select className="select" value={f.toolset_id} onChange={(e) => setF({ ...f, toolset_id: e.target.value })}><option value="">Select…</option>{(toolsets.data ?? []).map((ts) => <option key={ts.id} value={ts.id}>{ts.name}</option>)}</select></Field>
          )}
        </Drawer>
      )}
    </div>
  );
}
