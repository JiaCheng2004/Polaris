-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS vector;    -- for vector columns (pgvector)

--------------------------------------------------------------------------------
-- Cleanup (for development/testing only)
--------------------------------------------------------------------------------
DROP TABLE IF EXISTS attachments CASCADE;
DROP TABLE IF EXISTS messages CASCADE;
DROP TABLE IF EXISTS vector_store CASCADE;
DROP TABLE IF EXISTS threads CASCADE;

--------------------------------------------------------------------------------
-- Functions to generate prefixed UUIDs
--------------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION generate_prefixed_uuid(prefix text) 
RETURNS text AS $$
BEGIN
  RETURN prefix || '-' || gen_random_uuid()::text;
END;
$$ LANGUAGE plpgsql;

--------------------------------------------------------------------------------
-- Create threads table
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS threads (
    thread_id    TEXT PRIMARY KEY DEFAULT generate_prefixed_uuid('thread'),
    model        VARCHAR(255) NOT NULL,
    provider     VARCHAR(255) NOT NULL,
    tokens_spent BIGINT NOT NULL DEFAULT 0,
    cost         DECIMAL(12, 2) NOT NULL DEFAULT 0.00,  -- monetary cost
    purpose      VARCHAR(255) NOT NULL,                 -- e.g., "discord bot", "web app", etc.
    author       JSONB NOT NULL,                        -- JSON data describing the user(s)
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

--------------------------------------------------------------------------------
-- Create messages table
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS messages (
    message_id  TEXT PRIMARY KEY DEFAULT generate_prefixed_uuid('message'),
    thread_id   TEXT NOT NULL,
    role        VARCHAR(50) NOT NULL,   -- e.g. user | assistant | system
    content     JSONB NOT NULL,         -- flexible JSON for text blocks
    purpose     VARCHAR(255) NOT NULL,  -- e.g., "reply", "summary", "annotation"
    author      JSONB NOT NULL,         -- JSON representing who/what authored the message
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_messages_thread
      FOREIGN KEY(thread_id) 
      REFERENCES threads(thread_id) 
      ON DELETE CASCADE
);

--------------------------------------------------------------------------------
-- Create attachments table
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS attachments (
    file_id      TEXT PRIMARY KEY DEFAULT generate_prefixed_uuid('attachment'),
    message_id   TEXT NOT NULL,
    author       JSONB NOT NULL,                -- JSON describing who uploaded or generated it
    filename     TEXT NOT NULL,                 -- file name
    type         TEXT NOT NULL,                 -- MIME type (e.g. application/pdf, image/png)
    size         INT  NOT NULL,                 -- file size in bytes
    token_count  INT  NOT NULL DEFAULT 0,       -- tokens extracted (if applicable)
    content      TEXT,                          -- potentially long string
    metadata     JSONB NOT NULL DEFAULT '{}',    -- extra metadata
    content_hash TEXT NOT NULL,                 -- hash for integrity checks
    purpose      VARCHAR(255) NOT NULL,         -- e.g. "reference", "attachment", "embedded-image"
    parse_tool   JSONB NOT NULL DEFAULT '{}',    -- e.g. {"type": "api", "name": "gemini"}
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_attachments_message
      FOREIGN KEY(message_id)
      REFERENCES messages(message_id)
      ON DELETE CASCADE
);

--------------------------------------------------------------------------------
-- Create a vector store table for storing context/embeddings at the thread level
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS vector_store (
    vector_id   TEXT PRIMARY KEY DEFAULT generate_prefixed_uuid('vector'),
    thread_id   TEXT NOT NULL,
    embedding   VECTOR(3072) NOT NULL,  -- adjust dimension to match your model's embedding size
    metadata    JSONB NOT NULL DEFAULT '{}',
    embed_tool  JSONB NOT NULL DEFAULT '{}',    -- e.g. {"type": "api", "name": "gemini"} 
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_vector_store_thread
      FOREIGN KEY(thread_id)
      REFERENCES threads(thread_id)
      ON DELETE CASCADE
);

--------------------------------------------------------------------------------
-- Indexes to speed up lookups
--------------------------------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_messages_thread_id         ON messages (thread_id);
CREATE INDEX IF NOT EXISTS idx_attachments_message_id     ON attachments (message_id);
CREATE INDEX IF NOT EXISTS idx_vector_store_thread_id     ON vector_store (thread_id);

--------------------------------------------------------------------------------
-- Timestamp triggers to automatically update 'updated_at' on update
--------------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_threads_updated
BEFORE UPDATE ON threads
FOR EACH ROW
EXECUTE PROCEDURE set_updated_timestamp();

CREATE TRIGGER trg_messages_updated
BEFORE UPDATE ON messages
FOR EACH ROW
EXECUTE PROCEDURE set_updated_timestamp();

CREATE TRIGGER trg_attachments_updated
BEFORE UPDATE ON attachments
FOR EACH ROW
EXECUTE PROCEDURE set_updated_timestamp();

CREATE TRIGGER trg_vector_store_updated
BEFORE UPDATE ON vector_store
FOR EACH ROW
EXECUTE PROCEDURE set_updated_timestamp();

--------------------------------------------------------------------------------
-- Role and permission setup
--------------------------------------------------------------------------------

-- Create a dedicated "api" role for application access with CRUD
DO $$
BEGIN
   IF NOT EXISTS (
      SELECT FROM pg_catalog.pg_roles
      WHERE rolname = 'api'
   ) THEN
      CREATE ROLE api NOLOGIN;
   END IF;
END;
$$;

GRANT USAGE ON SCHEMA public TO api;
-- Grant full CRUD privileges on tables to api
GRANT SELECT, INSERT, UPDATE, DELETE ON threads, messages, attachments, vector_store TO api;

