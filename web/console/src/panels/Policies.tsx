import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createPolicy, listPolicies, listProjects } from "../api/resources";
import { useConn } from "../state/connection";
import { useToast } from "../components/Toast";
import { ErrorNote, Field, PageHeader, Panel, Spinner, Tag } from "../components/ui";
import { DataTable, type Column } from "../components/DataTable";
import { Drawer } from "../components/Modal";
import type { Policy } from "../api/types";

const csv = (v: string) => v.split(",").map((s) => s.trim()).filter(Boolean);

export function Policies() {
  const conn = useConn();
  const qc = useQueryClient();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ project_id: "", name: "", description: "", models: "", modalities: "", toolsets: "", mcp: "" });
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF((s) => ({ ...s, [k]: e.target.value }));

  const projects = useQuery({ queryKey: ["projects", conn.baseURL], queryFn: () => listProjects(conn) });
  const q = useQuery({ queryKey: ["policies", conn.baseURL], queryFn: () => listPolicies(conn) });
  const create = useMutation({
    mutationFn: () =>
      createPolicy(conn, {
        project_id: f.project_id,
        name: f.name,
        description: f.description,
        allowed_models: csv(f.models),
        allowed_modalities: csv(f.modalities),
        allowed_toolsets: csv(f.toolsets),
        allowed_mcp_bindings: csv(f.mcp),
      }),
    onSuccess: () => {
      toast.push("Policy created", "success");
      setOpen(false);
      setF({ project_id: "", name: "", description: "", models: "", modalities: "", toolsets: "", mcp: "" });
      void qc.invalidateQueries({ queryKey: ["policies", conn.baseURL] });
    },
    onError: (e) => toast.push((e as Error).message, "error"),
  });

  const cols: Column<Policy>[] = [
    { key: "name", header: "Name", render: (p) => <div>{p.name}<div className="num" style={{ fontSize: 11, color: "var(--text-3)" }}>{p.id}</div></div> },
    { key: "models", header: "Models", render: (p) => <Tag>{p.allowed_models.length || "all"}</Tag> },
    { key: "modalities", header: "Modalities", render: (p) => <span className="meta">{p.allowed_modalities.join(", ") || "all"}</span> },
    { key: "tools", header: "Toolsets / MCP", render: (p) => <span className="meta">{p.allowed_toolsets.length} / {p.allowed_mcp.length}</span> },
  ];

  return (
    <div>
      <PageHeader
        title="Policies"
        actions={
          <button className="btn btn-primary btn-sm" onClick={() => setOpen(true)} disabled={(projects.data ?? []).length === 0}>New policy</button>
        }
      />

      {q.isLoading && <Spinner />}
      {q.isError && <ErrorNote message={(q.error as Error).message} />}
      {q.data && <Panel><DataTable columns={cols} rows={q.data} rowKey={(p) => p.id} empty="No policies." /></Panel>}

      {open && (
        <Drawer title="New policy" onClose={() => setOpen(false)}
          footer={<><button className="btn btn-sm" onClick={() => setOpen(false)}>Cancel</button><button className="btn btn-primary btn-sm" disabled={!f.project_id || !f.name || create.isPending} onClick={() => create.mutate()}>{create.isPending ? "Creating…" : "Create"}</button></>}>
          <Field label="Project"><select className="select" value={f.project_id} onChange={set("project_id")}><option value="">Select…</option>{(projects.data ?? []).map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></Field>
          <Field label="Name"><input className="input" value={f.name} onChange={set("name")} /></Field>
          <Field label="Description"><input className="input" value={f.description} onChange={set("description")} /></Field>
          <Field label="Allowed models (comma)"><input className="input mono" value={f.models} onChange={set("models")} placeholder="openai/gpt-4o, anthropic/*" /></Field>
          <Field label="Allowed modalities (comma)"><input className="input mono" value={f.modalities} onChange={set("modalities")} placeholder="chat, embed" /></Field>
          <Field label="Allowed toolsets (comma)"><input className="input mono" value={f.toolsets} onChange={set("toolsets")} /></Field>
          <Field label="Allowed MCP bindings (comma)"><input className="input mono" value={f.mcp} onChange={set("mcp")} /></Field>
        </Drawer>
      )}
    </div>
  );
}
