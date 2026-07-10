import { request, type Conn } from "./http";
import {
  normalizeBudget,
  normalizeMCPBinding,
  normalizePolicy,
  normalizeTool,
  normalizeToolset,
} from "./normalize";
import type {
  AdminUsageReport,
  AuditListResponse,
  Budget,
  ListResponse,
  MCPBinding,
  Model,
  ModelListResponse,
  Policy,
  Project,
  ReadyResponse,
  ToolDefinition,
  Toolset,
  UsageGroupBy,
  UsageScope,
  VirtualKey,
} from "./types";

type Raw = Record<string, unknown>;
const dataOf = <T>(r: ListResponse<T> | undefined): T[] => (r?.data ?? []) as T[];

/* ---- Projects ---- */
export const listProjects = (c: Conn, includeArchived = false) =>
  request<ListResponse<Project>>(c, "/v1/projects", {
    query: { include_archived: includeArchived },
  }).then(dataOf);
export const createProject = (c: Conn, body: { name: string; description?: string }) =>
  request<Project>(c, "/v1/projects", { method: "POST", body });

/* ---- Virtual keys ---- */
export interface CreateVirtualKeyBody {
  project_id: string;
  owner_id?: string;
  name: string;
  rate_limit?: string;
  allowed_models?: string[];
  allowed_modalities?: string[];
  allowed_toolsets?: string[];
  allowed_mcp_bindings?: string[];
  is_admin?: boolean;
  expires_at?: string;
}
export const listVirtualKeys = (c: Conn, projectId?: string, includeRevoked = false) =>
  request<ListResponse<VirtualKey>>(c, "/v1/virtual_keys", {
    query: { project_id: projectId, include_revoked: includeRevoked },
  }).then(dataOf);
export const createVirtualKey = (c: Conn, body: CreateVirtualKeyBody) =>
  request<VirtualKey>(c, "/v1/virtual_keys", { method: "POST", body });
export const revokeVirtualKey = (c: Conn, id: string) =>
  request<void>(c, `/v1/virtual_keys/${encodeURIComponent(id)}`, { method: "DELETE" });

/* ---- Policies ---- */
export const listPolicies = (c: Conn, projectId?: string) =>
  request<ListResponse<Raw>>(c, "/v1/policies", { query: { project_id: projectId } }).then((r) =>
    dataOf(r).map(normalizePolicy)
  );
export const createPolicy = (
  c: Conn,
  body: {
    project_id: string;
    name: string;
    description?: string;
    allowed_models?: string[];
    allowed_modalities?: string[];
    allowed_toolsets?: string[];
    allowed_mcp_bindings?: string[];
  }
): Promise<Policy> =>
  request<Raw>(c, "/v1/policies", { method: "POST", body }).then(normalizePolicy);

/* ---- Budgets ---- */
export const listBudgets = (c: Conn, projectId?: string) =>
  request<ListResponse<Raw>>(c, "/v1/budgets", { query: { project_id: projectId } }).then((r) =>
    dataOf(r).map(normalizeBudget)
  );
export const createBudget = (
  c: Conn,
  body: {
    project_id: string;
    name: string;
    mode: Budget["mode"];
    limit_usd?: number;
    limit_requests?: number;
    window?: Budget["window"];
  }
): Promise<Budget> =>
  request<Raw>(c, "/v1/budgets", { method: "POST", body }).then(normalizeBudget);

/* ---- Tools / Toolsets / MCP ---- */
export const listTools = (c: Conn) =>
  request<ListResponse<Raw>>(c, "/v1/tools").then((r) => dataOf(r).map(normalizeTool));
export const createTool = (
  c: Conn,
  body: { name: string; description?: string; implementation: string; input_schema?: string }
): Promise<ToolDefinition> =>
  request<Raw>(c, "/v1/tools", { method: "POST", body }).then(normalizeTool);

export const listToolsets = (c: Conn) =>
  request<ListResponse<Raw>>(c, "/v1/toolsets").then((r) => dataOf(r).map(normalizeToolset));
export const createToolset = (
  c: Conn,
  body: { name: string; description?: string; tool_ids: string[] }
): Promise<Toolset> =>
  request<Raw>(c, "/v1/toolsets", { method: "POST", body }).then(normalizeToolset);

export const listMCPBindings = (c: Conn) =>
  request<ListResponse<Raw>>(c, "/v1/mcp/bindings").then((r) => dataOf(r).map(normalizeMCPBinding));
export const createMCPBinding = (
  c: Conn,
  body: {
    name: string;
    kind: MCPBinding["kind"];
    upstream_url?: string;
    toolset_id?: string;
    headers?: Record<string, string>;
  }
): Promise<MCPBinding> =>
  request<Raw>(c, "/v1/mcp/bindings", { method: "POST", body }).then(normalizeMCPBinding);

/* ---- Observability ---- */
export const getReady = (c: Conn) => request<ReadyResponse>(c, "/ready");
export const listModels = (c: Conn) =>
  request<ModelListResponse>(c, "/v1/models", { query: { include_aliases: true } }).then(
    (r): Model[] => r.data ?? []
  );

/* ---- Admin analytics (P1 endpoints) ---- */
export interface AdminUsageQuery {
  scope?: UsageScope;
  project_id?: string;
  key_id?: string;
  model?: string;
  modality?: string;
  provider?: string;
  from?: string;
  to?: string;
  group_by?: UsageGroupBy;
}
export const getAdminUsage = (c: Conn, q: AdminUsageQuery) =>
  request<AdminUsageReport>(c, "/v1/admin/usage", { query: { ...q } });

export interface AuditQuery {
  project_id?: string;
  actor_key_id?: string;
  kind?: string;
  resource_type?: string;
  from?: string;
  to?: string;
  limit?: number;
  cursor?: string;
}
export const listAuditEvents = (c: Conn, q: AuditQuery = {}) =>
  request<AuditListResponse>(c, "/v1/admin/audit-events", { query: { ...q } });
