// Wire types for the Polaris /v1 API, as actually serialized by the gateway
// (verified against the handlers). Control-plane responses that ship raw Go
// structs (PascalCase) are normalized to these snake_case shapes in normalize.ts.

export interface ErrorEnvelope {
  error: { message: string; type: string; code?: string; param?: string };
}

export interface ListResponse<T> {
  object: "list";
  data: T[];
}

export interface Project {
  id: string;
  name: string;
  description: string;
  created_at: string;
  archived_at: string | null;
}

export interface VirtualKey {
  id: string;
  project_id: string;
  name: string;
  key?: string; // raw value, returned ONCE on create
  key_prefix: string; // always "polaris-" — not a usable identifier
  rate_limit: string;
  allowed_models: string[];
  allowed_modalities: string[];
  allowed_toolsets: string[];
  allowed_mcp_bindings: string[];
  is_admin: boolean;
  created_at: string;
  last_used_at: string | null;
  expires_at: string | null;
  is_revoked: boolean;
}

export interface Policy {
  id: string;
  project_id: string;
  name: string;
  description: string;
  allowed_models: string[];
  allowed_modalities: string[];
  allowed_toolsets: string[];
  allowed_mcp: string[];
  created_at: string;
}

export type BudgetMode = "soft" | "hard";
export type BudgetWindow = "daily" | "monthly" | "lifetime";

export interface Budget {
  id: string;
  project_id: string;
  name: string;
  mode: BudgetMode;
  limit_usd: number;
  limit_requests: number;
  limit_file_bytes: number;
  limit_file_count: number;
  window: BudgetWindow;
  created_at: string;
}

export interface ToolDefinition {
  id: string;
  name: string;
  description: string;
  implementation: string;
  input_schema: string;
  enabled: boolean;
  created_at: string;
}

export interface Toolset {
  id: string;
  name: string;
  description: string;
  tool_ids: string[];
  created_at: string;
}

export type MCPBindingKind = "upstream_proxy" | "local_toolset";

export interface MCPBinding {
  id: string;
  name: string;
  kind: MCPBindingKind;
  upstream_url: string;
  toolset_id: string;
  headers_json: string; // stored upstream headers; redacted in the UI
  enabled: boolean;
  created_at: string;
}

export type BreakerState = "closed" | "open" | "half_open";

export interface HealthView {
  provider: string;
  samples: number;
  error_rate: number;
  ewma_latency_ms: number;
  health_score: number;
  breaker: BreakerState;
  inflight: number;
}

export interface ReadyResponse {
  status: "ready" | "not_ready" | "draining";
  store?: string;
  cache?: string;
  providers?: number;
  reliability?: HealthView[];
}

export interface ModelCapabilityFlags {
  chat?: boolean;
  vision?: boolean;
  file_understanding?: boolean;
  image_generation?: boolean;
  image_edit?: boolean;
  music_generation?: boolean;
  lyrics_generation?: boolean;
  video_generation?: boolean;
  tool_calling?: boolean;
  streaming?: boolean;
}

export interface Model {
  id: string;
  object: string;
  kind?: string;
  provider: string;
  provider_variant?: string;
  display_name?: string;
  family_id?: string;
  family_display_name?: string;
  status?: string;
  verification_class?: string;
  cost_tier?: string;
  latency_tier?: string;
  doc_url?: string;
  last_verified?: string;
  modality?: string;
  capabilities?: string[];
  aliases?: string[];
  capability_flags?: ModelCapabilityFlags;
  context_window?: number;
  max_output_tokens?: number;
  resolves_to?: string;
}

export interface ModelListResponse {
  object: string;
  data: Model[];
  routing?: unknown;
}

export interface AuditEvent {
  id: string;
  project_id: string;
  actor_key_id: string;
  kind: string;
  resource_type: string;
  resource_id: string;
  metadata_json: string;
  created_at: string;
}

export interface AuditListResponse {
  object: "list";
  data: AuditEvent[];
  next_cursor?: string;
}

// ---- Admin analytics (new endpoints, built in P1) ----
export type UsageScope = "global" | "project" | "key";
export type UsageGroupBy =
  | "day"
  | "model"
  | "provider"
  | "modality"
  | "status"
  | "token_source"
  | "cost_source";

export interface AdminUsageRow {
  key: string;
  requests: number;
  input_tokens: number;
  output_tokens: number;
  cached_input_tokens: number;
  cache_write_tokens: number;
  total_tokens: number;
  cost_usd: number;
  errors: number;
  avg_provider_latency_ms: number;
  avg_total_latency_ms: number;
}

export interface AdminUsageReport {
  from: string;
  to: string;
  scope: UsageScope;
  group_by: UsageGroupBy;
  totals: AdminUsageRow;
  rows: AdminUsageRow[];
}
