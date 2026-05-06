CREATE TABLE IF NOT EXISTS files (
    polaris_id        TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL,
    key_id            TEXT NOT NULL,
    sha256            TEXT NOT NULL,
    size              INTEGER NOT NULL,
    mime_type         TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    purpose           TEXT NOT NULL,
    origin_url        TEXT,
    inline_bytes      BLOB,
    blob_key          TEXT,
    metadata_json     TEXT NOT NULL DEFAULT '{}',
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at        TIMESTAMP,
    deleted_at        TIMESTAMP,
    FOREIGN KEY(project_id) REFERENCES projects(id)
);

CREATE INDEX IF NOT EXISTS idx_files_project_id ON files(project_id);
CREATE INDEX IF NOT EXISTS idx_files_sha256 ON files(sha256);
CREATE INDEX IF NOT EXISTS idx_files_expires_at ON files(expires_at);

CREATE TABLE IF NOT EXISTS file_provider_handles (
    polaris_id       TEXT NOT NULL,
    provider         TEXT NOT NULL,
    provider_file_id TEXT NOT NULL,
    purpose          TEXT NOT NULL,
    size_bytes       INTEGER NOT NULL,
    mime_type        TEXT NOT NULL,
    expires_at       TIMESTAMP,
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(polaris_id, provider),
    FOREIGN KEY(polaris_id) REFERENCES files(polaris_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_fph_provider ON file_provider_handles(provider, expires_at);

CREATE TABLE IF NOT EXISTS file_understanding_artifacts (
    sha256        TEXT NOT NULL,
    processor     TEXT NOT NULL,
    version       TEXT NOT NULL,
    mime_type     TEXT NOT NULL,
    text          TEXT NOT NULL,
    warning       TEXT,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    artifact_json TEXT NOT NULL DEFAULT '{}',
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(sha256, processor, version)
);

CREATE INDEX IF NOT EXISTS idx_fua_sha256 ON file_understanding_artifacts(sha256);
