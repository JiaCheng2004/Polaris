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

CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX idx_api_keys_owner_id ON api_keys(owner_id);

CREATE TABLE IF NOT EXISTS request_logs (
    id                   TEXT PRIMARY KEY,
    request_id           TEXT NOT NULL,
    key_id               TEXT NOT NULL,
    model                TEXT NOT NULL,
    modality             TEXT NOT NULL,
    provider_latency_ms  INTEGER,
    total_latency_ms     INTEGER,
    input_tokens         INTEGER,
    output_tokens        INTEGER,
    total_tokens         INTEGER,
    estimated_cost       DOUBLE PRECISION,
    status_code          INTEGER NOT NULL,
    error_type           TEXT,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_request_logs_key_id ON request_logs(key_id);
CREATE INDEX idx_request_logs_model ON request_logs(model);
CREATE INDEX idx_request_logs_created_at ON request_logs(created_at);
CREATE INDEX idx_request_logs_modality ON request_logs(modality);
