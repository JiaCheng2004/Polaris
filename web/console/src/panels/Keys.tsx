import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createVirtualKey, listProjects, listVirtualKeys, revokeVirtualKey } from "../api/resources";
import { useConn } from "../state/connection";
import { useToast } from "../components/Toast";
import { ErrorNote, PageHeader, Panel, Spinner, Tag } from "../components/ui";
import { DataTable, type Column } from "../components/DataTable";
import { ConfirmDialog, Drawer, Modal } from "../components/Modal";
import type { VirtualKey } from "../api/types";

function csv(v: string): string[] {
  return v.split(",").map((s) => s.trim()).filter(Boolean);
}

export function Keys() {
  const conn = useConn();
  const qc = useQueryClient();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [revealed, setRevealed] = useState<VirtualKey | null>(null);
  const [revoke, setRevoke] = useState<VirtualKey | null>(null);
  const [includeRevoked, setIncludeRevoked] = useState(false);

  const [projectId, setProjectId] = useState("");
  const [name, setName] = useState("");
  const [rateLimit, setRateLimit] = useState("");
  const [allowedModels, setAllowedModels] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);

  const projects = useQuery({ queryKey: ["projects", conn.baseURL], queryFn: () => listProjects(conn) });
  const q = useQuery({
    queryKey: ["vkeys", conn.baseURL, includeRevoked],
    queryFn: () => listVirtualKeys(conn, undefined, includeRevoked),
  });

  const create = useMutation({
    mutationFn: () =>
      createVirtualKey(conn, {
        project_id: projectId,
        name,
        rate_limit: rateLimit || undefined,
        allowed_models: allowedModels ? csv(allowedModels) : undefined,
        is_admin: isAdmin,
      }),
    onSuccess: (key) => {
      setOpen(false);
      setRevealed(key); // show-once reveal of the raw key value
      setName("");
      setRateLimit("");
      setAllowedModels("");
      setIsAdmin(false);
      void qc.invalidateQueries({ queryKey: ["vkeys", conn.baseURL] });
    },
    onError: (e) => toast.push((e as Error).message, "error"),
  });

  const doRevoke = useMutation({
    mutationFn: (id: string) => revokeVirtualKey(conn, id),
    onSuccess: () => {
      toast.push("Key revoked", "success");
      void qc.invalidateQueries({ queryKey: ["vkeys", conn.baseURL] });
    },
    onError: (e) => toast.push((e as Error).message, "error"),
  });

  const cols: Column<VirtualKey>[] = [
    { key: "name", header: "Name", render: (k) => (
        <div>
          <div>{k.name}</div>
          <div className="num" style={{ fontSize: 11, color: "var(--text-3)" }}>{k.id}</div>
        </div>
      ) },
    { key: "admin", header: "Role", render: (k) => (k.is_admin ? <Tag tone="accent">admin</Tag> : <Tag>member</Tag>) },
    { key: "rate", header: "Rate limit", mono: true, render: (k) => k.rate_limit || "—" },
    { key: "used", header: "Last used", mono: true, render: (k) => (k.last_used_at ? new Date(k.last_used_at).toLocaleDateString() : "never") },
    { key: "state", header: "State", render: (k) => (k.is_revoked ? <Tag>revoked</Tag> : <span style={{ color: "var(--status-good)" }}>active</span>) },
    { key: "act", header: "", align: "right", render: (k) =>
        !k.is_revoked ? (
          <button className="btn btn-danger btn-sm" onClick={() => setRevoke(k)}>Revoke</button>
        ) : null },
  ];

  return (
    <div>
      <PageHeader
        title="Virtual Keys"
        actions={
          <button className="btn btn-primary btn-sm" onClick={() => setOpen(true)} disabled={(projects.data ?? []).length === 0}>
            Issue key
          </button>
        }
      />

      <label className="meta" style={{ display: "inline-flex", gap: 6, marginBottom: 12, cursor: "pointer", textTransform: "none" }}>
        <input type="checkbox" checked={includeRevoked} onChange={(e) => setIncludeRevoked(e.target.checked)} /> show revoked
      </label>

      {q.isLoading && <Spinner />}
      {q.isError && <ErrorNote message={(q.error as Error).message} />}
      {q.data && (
        <Panel>
          <DataTable columns={cols} rows={q.data} rowKey={(k) => k.id} empty={(projects.data ?? []).length === 0 ? "Create a project first, then issue keys." : "No keys yet."} />
        </Panel>
      )}

      {open && (
        <Drawer
          title="Issue virtual key"
          onClose={() => setOpen(false)}
          footer={
            <>
              <button className="btn btn-sm" onClick={() => setOpen(false)}>Cancel</button>
              <button className="btn btn-primary btn-sm" disabled={!projectId || !name || create.isPending} onClick={() => create.mutate()}>
                {create.isPending ? "Issuing…" : "Issue"}
              </button>
            </>
          }
        >
          <div className="field" style={{ marginBottom: 14 }}>
            <label>Project</label>
            <select className="select" value={projectId} onChange={(e) => setProjectId(e.target.value)}>
              <option value="">Select…</option>
              {(projects.data ?? []).map((p) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
          </div>
          <div className="field" style={{ marginBottom: 14 }}>
            <label>Name</label>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="field" style={{ marginBottom: 14 }}>
            <label>Rate limit (optional, e.g. 60/min)</label>
            <input className="input mono" value={rateLimit} onChange={(e) => setRateLimit(e.target.value)} placeholder="60/min" />
          </div>
          <div className="field" style={{ marginBottom: 14 }}>
            <label>Allowed models (comma-separated, optional)</label>
            <input className="input mono" value={allowedModels} onChange={(e) => setAllowedModels(e.target.value)} placeholder="openai/gpt-4o, anthropic/*" />
          </div>
          <label style={{ display: "flex", gap: 8, alignItems: "center", fontSize: 13 }}>
            <input type="checkbox" checked={isAdmin} onChange={(e) => setIsAdmin(e.target.checked)} /> Grant admin (control-plane access)
          </label>
        </Drawer>
      )}

      {revealed && (
        <Modal title="Copy your key now" onClose={() => setRevealed(null)} width={520}
          footer={<button className="btn btn-primary btn-sm" onClick={() => setRevealed(null)}>Done</button>}>
          <p style={{ fontSize: 13, color: "var(--text-2)", marginTop: 0 }}>
            This is the only time the full key is shown. Store it securely — it cannot be recovered.
          </p>
          <div style={{ display: "flex", gap: 8 }}>
            <input className="input mono" readOnly value={revealed.key ?? ""} style={{ flex: 1 }} onFocus={(e) => e.currentTarget.select()} />
            <button className="btn btn-sm" onClick={() => { void navigator.clipboard?.writeText(revealed.key ?? ""); toast.push("Copied", "success"); }}>
              Copy
            </button>
          </div>
        </Modal>
      )}

      {revoke && (
        <ConfirmDialog
          title="Revoke key"
          danger
          confirmLabel="Revoke"
          body={<>Revoke <strong>{revoke.name}</strong>? Requests using it will fail immediately. This cannot be undone.</>}
          onConfirm={() => doRevoke.mutate(revoke.id)}
          onClose={() => setRevoke(null)}
        />
      )}
    </div>
  );
}
