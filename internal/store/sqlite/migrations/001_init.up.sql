CREATE TABLE IF NOT EXISTS api_keys (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    key_hash        TEXT NOT NULL UNIQUE,
    key_prefix      TEXT NOT NULL,
    owner_id        TEXT,
    rate_limit      TEXT,
    allowed_models  TEXT NOT NULL DEFAULT '["*"]',
    is_admin        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at    TIMESTAMP,
    expires_at      TIMESTAMP,
    is_revoked      BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_owner_id ON api_keys(owner_id);

CREATE TABLE IF NOT EXISTS request_logs (
    id                   TEXT PRIMARY KEY,
    request_id           TEXT NOT NULL,
    key_id               TEXT NOT NULL,
    project_id           TEXT,
    model                TEXT NOT NULL,
    modality             TEXT NOT NULL,
    interface_family     TEXT,
    token_source         TEXT,
    cache_status         TEXT,
    fallback_model       TEXT,
    trace_id             TEXT,
    toolset              TEXT,
    mcp_binding          TEXT,
    provider_latency_ms  INTEGER,
    total_latency_ms     INTEGER,
    input_tokens         INTEGER,
    output_tokens        INTEGER,
    total_tokens         INTEGER,
    estimated_cost       REAL,
    status_code          INTEGER NOT NULL,
    error_type           TEXT,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_request_logs_key_id ON request_logs(key_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_project_id ON request_logs(project_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_model ON request_logs(model);
CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_modality ON request_logs(modality);

CREATE TABLE IF NOT EXISTS projects (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    description   TEXT,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at   TIMESTAMP
);

CREATE TABLE IF NOT EXISTS virtual_keys (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL,
    name                TEXT NOT NULL,
    key_hash            TEXT NOT NULL UNIQUE,
    key_prefix          TEXT NOT NULL,
    rate_limit          TEXT,
    allowed_models      TEXT NOT NULL DEFAULT '["*"]',
    allowed_modalities  TEXT NOT NULL DEFAULT '[]',
    allowed_toolsets    TEXT NOT NULL DEFAULT '[]',
    allowed_mcp         TEXT NOT NULL DEFAULT '[]',
    is_admin            BOOLEAN NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at        TIMESTAMP,
    expires_at          TIMESTAMP,
    is_revoked          BOOLEAN NOT NULL DEFAULT FALSE,
    FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE INDEX IF NOT EXISTS idx_virtual_keys_key_hash ON virtual_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_virtual_keys_project_id ON virtual_keys(project_id);

CREATE TABLE IF NOT EXISTS policies (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL,
    name                TEXT NOT NULL,
    description         TEXT,
    allowed_models      TEXT NOT NULL DEFAULT '["*"]',
    allowed_modalities  TEXT NOT NULL DEFAULT '[]',
    allowed_toolsets    TEXT NOT NULL DEFAULT '[]',
    allowed_mcp         TEXT NOT NULL DEFAULT '[]',
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS budgets (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    name            TEXT NOT NULL,
    mode            TEXT NOT NULL,
    limit_usd       REAL NOT NULL DEFAULT 0,
    limit_requests  INTEGER NOT NULL DEFAULT 0,
    window          TEXT NOT NULL DEFAULT 'monthly',
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS audit_events (
    id             TEXT PRIMARY KEY,
    project_id     TEXT,
    actor_key_id   TEXT,
    kind           TEXT NOT NULL,
    resource_type  TEXT NOT NULL,
    resource_id    TEXT NOT NULL,
    metadata_json  TEXT NOT NULL DEFAULT '{}',
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_events_project_id ON audit_events(project_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_kind ON audit_events(kind);

CREATE TABLE IF NOT EXISTS tool_definitions (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,
    description      TEXT,
    implementation   TEXT NOT NULL,
    input_schema     TEXT NOT NULL DEFAULT '{}',
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS toolsets (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,
    description      TEXT,
    tool_ids         TEXT NOT NULL DEFAULT '[]',
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS mcp_bindings (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,
    kind             TEXT NOT NULL,
    upstream_url     TEXT,
    toolset_id       TEXT,
    headers_json     TEXT NOT NULL DEFAULT '{}',
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(toolset_id) REFERENCES toolsets(id)
);

CREATE TABLE IF NOT EXISTS archived_voices (
    provider      TEXT NOT NULL,
    model         TEXT NOT NULL DEFAULT '',
    voice_id      TEXT NOT NULL,
    archived_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (provider, model, voice_id)
);

CREATE INDEX IF NOT EXISTS idx_archived_voices_provider_model ON archived_voices(provider, model);
