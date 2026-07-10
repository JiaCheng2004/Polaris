// The gateway serializes policies/budgets/tools/toolsets/mcp-bindings as raw Go
// structs (PascalCase, no json tags) while projects/keys use snake_case. These
// helpers read either casing so the UI works today and keeps working if the API
// is later made consistent.
import type { Budget, MCPBinding, Policy, ToolDefinition, Toolset } from "./types";

type Raw = Record<string, unknown>;

function pick<T>(o: Raw, keys: string[], fallback: T): T {
  for (const k of keys) {
    if (o[k] !== undefined && o[k] !== null) return o[k] as T;
  }
  return fallback;
}
const str = (o: Raw, ...k: string[]) => pick<string>(o, k, "");
const num = (o: Raw, ...k: string[]) => pick<number>(o, k, 0);
const bool = (o: Raw, ...k: string[]) => pick<boolean>(o, k, false);
const arr = (o: Raw, ...k: string[]) => pick<string[]>(o, k, []) ?? [];

export function normalizePolicy(o: Raw): Policy {
  return {
    id: str(o, "id", "ID"),
    project_id: str(o, "project_id", "ProjectID"),
    name: str(o, "name", "Name"),
    description: str(o, "description", "Description"),
    allowed_models: arr(o, "allowed_models", "AllowedModels"),
    allowed_modalities: arr(o, "allowed_modalities", "AllowedModalities"),
    allowed_toolsets: arr(o, "allowed_toolsets", "AllowedToolsets"),
    allowed_mcp: arr(o, "allowed_mcp", "allowed_mcp_bindings", "AllowedMCP"),
    created_at: str(o, "created_at", "CreatedAt"),
  };
}

export function normalizeBudget(o: Raw): Budget {
  return {
    id: str(o, "id", "ID"),
    project_id: str(o, "project_id", "ProjectID"),
    name: str(o, "name", "Name"),
    mode: (str(o, "mode", "Mode") || "soft") as Budget["mode"],
    limit_usd: num(o, "limit_usd", "LimitUSD"),
    limit_requests: num(o, "limit_requests", "LimitRequests"),
    limit_file_bytes: num(o, "limit_file_bytes", "LimitFileBytes"),
    limit_file_count: num(o, "limit_file_count", "LimitFileCount"),
    window: (str(o, "window", "Window") || "monthly") as Budget["window"],
    created_at: str(o, "created_at", "CreatedAt"),
  };
}

export function normalizeTool(o: Raw): ToolDefinition {
  return {
    id: str(o, "id", "ID"),
    name: str(o, "name", "Name"),
    description: str(o, "description", "Description"),
    implementation: str(o, "implementation", "Implementation"),
    input_schema: str(o, "input_schema", "InputSchema"),
    enabled: bool(o, "enabled", "Enabled"),
    created_at: str(o, "created_at", "CreatedAt"),
  };
}

export function normalizeToolset(o: Raw): Toolset {
  return {
    id: str(o, "id", "ID"),
    name: str(o, "name", "Name"),
    description: str(o, "description", "Description"),
    tool_ids: arr(o, "tool_ids", "ToolIDs"),
    created_at: str(o, "created_at", "CreatedAt"),
  };
}

export function normalizeMCPBinding(o: Raw): MCPBinding {
  return {
    id: str(o, "id", "ID"),
    name: str(o, "name", "Name"),
    kind: (str(o, "kind", "Kind") || "upstream_proxy") as MCPBinding["kind"],
    upstream_url: str(o, "upstream_url", "UpstreamURL"),
    toolset_id: str(o, "toolset_id", "ToolsetID"),
    headers_json: str(o, "headers_json", "HeadersJSON"),
    enabled: bool(o, "enabled", "Enabled"),
    created_at: str(o, "created_at", "CreatedAt"),
  };
}
