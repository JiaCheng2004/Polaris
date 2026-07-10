import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createProject, listProjects } from "../api/resources";
import { useConn } from "../state/connection";
import { useToast } from "../components/Toast";
import { ErrorNote, PageHeader, Panel, Spinner } from "../components/ui";
import { DataTable, type Column } from "../components/DataTable";
import { Drawer } from "../components/Modal";
import type { Project } from "../api/types";

export function Projects() {
  const conn = useConn();
  const qc = useQueryClient();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");

  const q = useQuery({ queryKey: ["projects", conn.baseURL], queryFn: () => listProjects(conn) });
  const create = useMutation({
    mutationFn: () => createProject(conn, { name, description }),
    onSuccess: () => {
      toast.push("Project created", "success");
      setOpen(false);
      setName("");
      setDescription("");
      void qc.invalidateQueries({ queryKey: ["projects", conn.baseURL] });
    },
    onError: (e) => toast.push((e as Error).message, "error"),
  });

  const cols: Column<Project>[] = [
    { key: "id", header: "ID", mono: true, render: (p) => <span style={{ fontSize: 12 }}>{p.id}</span> },
    { key: "name", header: "Name", render: (p) => p.name },
    { key: "desc", header: "Description", render: (p) => <span style={{ color: "var(--text-3)" }}>{p.description || "—"}</span> },
    { key: "created", header: "Created", mono: true, render: (p) => new Date(p.created_at).toLocaleDateString() },
  ];

  return (
    <div>
      <PageHeader
        title="Projects"
        actions={
          <button className="btn btn-primary btn-sm" onClick={() => setOpen(true)}>
            New project
          </button>
        }
      />

      {q.isLoading && <Spinner />}
      {q.isError && <ErrorNote message={(q.error as Error).message} />}
      {q.data && (
        <Panel>
          <DataTable columns={cols} rows={q.data} rowKey={(p) => p.id} empty="Create your first project." />
        </Panel>
      )}

      {open && (
        <Drawer
          title="New project"
          onClose={() => setOpen(false)}
          footer={
            <>
              <button className="btn btn-sm" onClick={() => setOpen(false)}>Cancel</button>
              <button className="btn btn-primary btn-sm" disabled={!name || create.isPending} onClick={() => create.mutate()}>
                {create.isPending ? "Creating…" : "Create"}
              </button>
            </>
          }
        >
          <div className="field" style={{ marginBottom: 14 }}>
            <label>Name</label>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          </div>
          <div className="field">
            <label>Description</label>
            <input className="input" value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
        </Drawer>
      )}
    </div>
  );
}
