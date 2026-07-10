import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createBudget, listBudgets, listProjects } from "../api/resources";
import { useConn } from "../state/connection";
import { useToast } from "../components/Toast";
import { ErrorNote, Field, PageHeader, Panel, Spinner, Tag } from "../components/ui";
import { DataTable, type Column } from "../components/DataTable";
import { Drawer } from "../components/Modal";
import { fmtUSD } from "../viz/charts";
import type { Budget } from "../api/types";

export function Budgets() {
  const conn = useConn();
  const qc = useQueryClient();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ project_id: "", name: "", mode: "soft" as Budget["mode"], limit_usd: "", limit_requests: "", window: "monthly" as Budget["window"] });

  const projects = useQuery({ queryKey: ["projects", conn.baseURL], queryFn: () => listProjects(conn) });
  const q = useQuery({ queryKey: ["budgets", conn.baseURL], queryFn: () => listBudgets(conn) });
  const create = useMutation({
    mutationFn: () =>
      createBudget(conn, {
        project_id: f.project_id,
        name: f.name,
        mode: f.mode,
        limit_usd: f.limit_usd ? Number(f.limit_usd) : undefined,
        limit_requests: f.limit_requests ? Number(f.limit_requests) : undefined,
        window: f.window,
      }),
    onSuccess: () => {
      toast.push("Budget created", "success");
      setOpen(false);
      setF({ project_id: "", name: "", mode: "soft", limit_usd: "", limit_requests: "", window: "monthly" });
      void qc.invalidateQueries({ queryKey: ["budgets", conn.baseURL] });
    },
    onError: (e) => toast.push((e as Error).message, "error"),
  });

  const cols: Column<Budget>[] = [
    { key: "name", header: "Name", render: (b) => <div>{b.name}<div className="num" style={{ fontSize: 11, color: "var(--text-3)" }}>{b.id}</div></div> },
    { key: "mode", header: "Mode", render: (b) => <Tag tone={b.mode === "hard" ? "accent" : "muted"}>{b.mode}</Tag> },
    { key: "usd", header: "USD limit", align: "right", mono: true, render: (b) => (b.limit_usd > 0 ? fmtUSD(b.limit_usd) : "—") },
    { key: "req", header: "Request limit", align: "right", mono: true, render: (b) => (b.limit_requests > 0 ? b.limit_requests.toLocaleString() : "—") },
    { key: "win", header: "Window", render: (b) => <span className="meta">{b.window}</span> },
  ];

  return (
    <div>
      <PageHeader
        title="Budgets"
        actions={
          <button className="btn btn-primary btn-sm" onClick={() => setOpen(true)} disabled={(projects.data ?? []).length === 0}>New budget</button>
        }
      />

      {q.isLoading && <Spinner />}
      {q.isError && <ErrorNote message={(q.error as Error).message} />}
      {q.data && <Panel><DataTable columns={cols} rows={q.data} rowKey={(b) => b.id} empty="No budgets." /></Panel>}

      {open && (
        <Drawer title="New budget" onClose={() => setOpen(false)}
          footer={<><button className="btn btn-sm" onClick={() => setOpen(false)}>Cancel</button><button className="btn btn-primary btn-sm" disabled={!f.project_id || !f.name || create.isPending} onClick={() => create.mutate()}>{create.isPending ? "Creating…" : "Create"}</button></>}>
          <Field label="Project"><select className="select" value={f.project_id} onChange={(e) => setF({ ...f, project_id: e.target.value })}><option value="">Select…</option>{(projects.data ?? []).map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></Field>
          <Field label="Name"><input className="input" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Mode"><select className="select" value={f.mode} onChange={(e) => setF({ ...f, mode: e.target.value as Budget["mode"] })}><option value="soft">soft (warn)</option><option value="hard">hard (block)</option></select></Field>
          <Field label="USD limit (optional)"><input className="input mono" type="number" value={f.limit_usd} onChange={(e) => setF({ ...f, limit_usd: e.target.value })} placeholder="25" /></Field>
          <Field label="Request limit (optional)"><input className="input mono" type="number" value={f.limit_requests} onChange={(e) => setF({ ...f, limit_requests: e.target.value })} placeholder="100000" /></Field>
          <Field label="Window"><select className="select" value={f.window} onChange={(e) => setF({ ...f, window: e.target.value as Budget["window"] })}><option value="daily">daily</option><option value="monthly">monthly</option><option value="lifetime">lifetime</option></select></Field>
        </Drawer>
      )}
    </div>
  );
}
