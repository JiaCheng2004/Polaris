import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createTool, createToolset, listTools, listToolsets } from "../api/resources";
import { useConn } from "../state/connection";
import { useToast } from "../components/Toast";
import { ErrorNote, Field, PageHeader, Panel, Spinner, Tag } from "../components/ui";
import { DataTable, type Column } from "../components/DataTable";
import { Drawer } from "../components/Modal";
import type { ToolDefinition, Toolset } from "../api/types";

export function Tools() {
  const conn = useConn();
  const qc = useQueryClient();
  const toast = useToast();
  const [openTool, setOpenTool] = useState(false);
  const [openSet, setOpenSet] = useState(false);
  const [t, setT] = useState({ name: "", description: "", implementation: "", input_schema: "" });
  const [s, setS] = useState<{ name: string; description: string; tool_ids: string[] }>({ name: "", description: "", tool_ids: [] });

  const tools = useQuery({ queryKey: ["tools", conn.baseURL], queryFn: () => listTools(conn) });
  const toolsets = useQuery({ queryKey: ["toolsets", conn.baseURL], queryFn: () => listToolsets(conn) });

  const createT = useMutation({
    mutationFn: () => createTool(conn, { name: t.name, description: t.description, implementation: t.implementation, input_schema: t.input_schema || undefined }),
    onSuccess: () => { toast.push("Tool registered", "success"); setOpenTool(false); setT({ name: "", description: "", implementation: "", input_schema: "" }); void qc.invalidateQueries({ queryKey: ["tools", conn.baseURL] }); },
    onError: (e) => toast.push((e as Error).message, "error"),
  });
  const createS = useMutation({
    mutationFn: () => createToolset(conn, { name: s.name, description: s.description, tool_ids: s.tool_ids }),
    onSuccess: () => { toast.push("Toolset created", "success"); setOpenSet(false); setS({ name: "", description: "", tool_ids: [] }); void qc.invalidateQueries({ queryKey: ["toolsets", conn.baseURL] }); },
    onError: (e) => toast.push((e as Error).message, "error"),
  });

  const toolCols: Column<ToolDefinition>[] = [
    { key: "name", header: "Name", render: (x) => <div>{x.name}<div className="num" style={{ fontSize: 11, color: "var(--text-3)" }}>{x.id}</div></div> },
    { key: "impl", header: "Implementation", mono: true, render: (x) => x.implementation },
    { key: "desc", header: "Description", render: (x) => <span style={{ color: "var(--text-3)" }}>{x.description || "—"}</span> },
    { key: "en", header: "Enabled", render: (x) => (x.enabled ? <span style={{ color: "var(--status-good)" }}>yes</span> : <Tag>no</Tag>) },
  ];
  const setCols: Column<Toolset>[] = [
    { key: "name", header: "Name", render: (x) => <div>{x.name}<div className="num" style={{ fontSize: 11, color: "var(--text-3)" }}>{x.id}</div></div> },
    { key: "tools", header: "Tools", render: (x) => <Tag>{x.tool_ids.length}</Tag> },
    { key: "desc", header: "Description", render: (x) => <span style={{ color: "var(--text-3)" }}>{x.description || "—"}</span> },
  ];

  return (
    <div>
      <PageHeader title="Tools & Toolsets" />

      <div className="grid">
        <Panel className="col-12" title="Tools" reveal actions={<button className="btn btn-primary btn-sm" onClick={() => setOpenTool(true)}>Register tool</button>}>
          {tools.isLoading && <Spinner />}
          {tools.isError && <ErrorNote message={(tools.error as Error).message} />}
          {tools.data && <DataTable columns={toolCols} rows={tools.data} rowKey={(x) => x.id} empty="No tools registered." />}
        </Panel>
        <Panel className="col-12" title="Toolsets" reveal actions={<button className="btn btn-primary btn-sm" onClick={() => setOpenSet(true)} disabled={(tools.data ?? []).length === 0}>New toolset</button>}>
          {toolsets.isLoading && <Spinner />}
          {toolsets.data && <DataTable columns={setCols} rows={toolsets.data} rowKey={(x) => x.id} empty="No toolsets." />}
        </Panel>
      </div>

      {openTool && (
        <Drawer title="Register tool" onClose={() => setOpenTool(false)}
          footer={<><button className="btn btn-sm" onClick={() => setOpenTool(false)}>Cancel</button><button className="btn btn-primary btn-sm" disabled={!t.name || !t.implementation || createT.isPending} onClick={() => createT.mutate()}>{createT.isPending ? "Saving…" : "Register"}</button></>}>
          <Field label="Name"><input className="input" value={t.name} onChange={(e) => setT({ ...t, name: e.target.value })} /></Field>
          <Field label="Implementation"><input className="input mono" value={t.implementation} onChange={(e) => setT({ ...t, implementation: e.target.value })} placeholder="echo" /></Field>
          <Field label="Description"><input className="input" value={t.description} onChange={(e) => setT({ ...t, description: e.target.value })} /></Field>
          <Field label="Input schema (JSON, optional)"><textarea className="textarea" value={t.input_schema} onChange={(e) => setT({ ...t, input_schema: e.target.value })} placeholder='{"type":"object"}' /></Field>
        </Drawer>
      )}

      {openSet && (
        <Drawer title="New toolset" onClose={() => setOpenSet(false)}
          footer={<><button className="btn btn-sm" onClick={() => setOpenSet(false)}>Cancel</button><button className="btn btn-primary btn-sm" disabled={!s.name || s.tool_ids.length === 0 || createS.isPending} onClick={() => createS.mutate()}>{createS.isPending ? "Saving…" : "Create"}</button></>}>
          <Field label="Name"><input className="input" value={s.name} onChange={(e) => setS({ ...s, name: e.target.value })} /></Field>
          <Field label="Description"><input className="input" value={s.description} onChange={(e) => setS({ ...s, description: e.target.value })} /></Field>
          <Field label="Tools">
            <div style={{ display: "flex", flexDirection: "column", gap: 6, maxHeight: 220, overflowY: "auto" }}>
              {(tools.data ?? []).map((tool) => (
                <label key={tool.id} style={{ display: "flex", gap: 8, fontSize: 13, alignItems: "center" }}>
                  <input type="checkbox" checked={s.tool_ids.includes(tool.id)} onChange={(e) => setS((prev) => ({ ...prev, tool_ids: e.target.checked ? [...prev.tool_ids, tool.id] : prev.tool_ids.filter((x) => x !== tool.id) }))} />
                  {tool.name} <span className="meta">{tool.implementation}</span>
                </label>
              ))}
            </div>
          </Field>
        </Drawer>
      )}
    </div>
  );
}
