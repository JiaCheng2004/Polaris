-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS vector;    -- for vector columns (pgvector)

--------------------------------------------------------------------------------
-- Cleanup (for development/testing only)
--------------------------------------------------------------------------------
DROP TABLE IF EXISTS message_files CASCADE;
DROP TABLE IF EXISTS files CASCADE;
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
    author      JSONB NOT NULL,         -- JSON representing who/what authored the message
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_messages_thread
      FOREIGN KEY(thread_id) 
      REFERENCES threads(thread_id) 
      ON DELETE CASCADE
);

--------------------------------------------------------------------------------
-- Create files table
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS files (
    file_id      TEXT PRIMARY KEY DEFAULT generate_prefixed_uuid('file'),
    author       JSONB NOT NULL,                -- JSON describing who uploaded or generated it
    filename     TEXT NOT NULL,                 -- file name
    type         TEXT NOT NULL,                 -- MIME type (e.g. application/pdf, image/png)
    size         INT  NOT NULL,                 -- file size in bytes
    token_count  INT  NOT NULL DEFAULT 0,       -- tokens extracted (if applicable)
    content      TEXT,                          -- potentially long string
    metadata     JSONB NOT NULL DEFAULT '{}',   -- extra metadata
    content_hash TEXT NOT NULL,                 -- hash for integrity checks
    address      TEXT NOT NULL,                 -- path to physical file or "deleted" if removed
    parse_tool   JSONB NOT NULL DEFAULT '{}',   -- e.g. {"type": "api", "name": "gemini"}
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

--------------------------------------------------------------------------------
-- Create message_files junction table for message-file relationships
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS message_files (
    message_id  TEXT NOT NULL,
    file_id     TEXT NOT NULL,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    PRIMARY KEY (message_id, file_id),
    
    CONSTRAINT fk_message_files_message
      FOREIGN KEY(message_id)
      REFERENCES messages(message_id)
      ON DELETE CASCADE,
      
    CONSTRAINT fk_message_files_file
      FOREIGN KEY(file_id)
      REFERENCES files(file_id)
      ON DELETE CASCADE
);

--------------------------------------------------------------------------------
-- Create a vector store table for storing context/embeddings at the thread level
--------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS vector_store (
    vector_id   TEXT PRIMARY KEY DEFAULT generate_prefixed_uuid('vector'),
    thread_id   TEXT NOT NULL,
    embedding   VECTOR(3072) NOT NULL,  -- adjust dimension to match your model's embedding size
    content     TEXT NOT NULL,          -- original chunked text that was embedded
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
CREATE INDEX IF NOT EXISTS idx_messages_thread_id     ON messages (thread_id);
CREATE INDEX IF NOT EXISTS idx_vector_store_thread_id ON vector_store (thread_id);
CREATE INDEX IF NOT EXISTS idx_files_content_hash     ON files (content_hash);
CREATE INDEX IF NOT EXISTS idx_message_files_file_id  ON message_files (file_id);

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

CREATE TRIGGER trg_files_updated
BEFORE UPDATE ON files
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
GRANT SELECT, INSERT, UPDATE, DELETE ON threads, messages, files, vector_store, message_files TO api;

--------------------------------------------------------------------------------
-- Define search_vectors function for vector similarity search
--------------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION search_vectors(
    query_embedding VECTOR(3072),
    namespace TEXT,
    thread_id_param TEXT,
    similarity_threshold FLOAT,
    match_count INT
) RETURNS TABLE (
    vector_id TEXT,
    thread_id TEXT,
    content TEXT,
    metadata JSONB,
    similarity FLOAT
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        vs.vector_id,
        vs.thread_id,
        vs.content,
        vs.metadata,
        1 - (vs.embedding <=> query_embedding) AS similarity
    FROM 
        vector_store vs
    WHERE 
        vs.thread_id = thread_id_param
        AND vs.metadata->>'namespace' = namespace
        AND (1 - (vs.embedding <=> query_embedding)) > similarity_threshold
    ORDER BY 
        vs.embedding <=> query_embedding
    LIMIT 
        match_count;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER VOLATILE;

-- Grant execute permission to the api role
GRANT EXECUTE ON FUNCTION search_vectors(VECTOR(3072), TEXT, TEXT, FLOAT, INT) TO api;
COMMENT ON FUNCTION search_vectors(VECTOR(3072), TEXT, TEXT, FLOAT, INT) 
IS 'Search for vectors in vector_store by similarity';

--------------------------------------------------------------------------------
-- Define a simpler vector retrieval function as fallback
--------------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION get_thread_vectors(
    thread_id_param TEXT,
    namespace_param TEXT,
    limit_param INT DEFAULT 10
) RETURNS TABLE (
    vector_id TEXT,
    thread_id TEXT,
    content TEXT,
    metadata JSONB,
    embedding VECTOR(3072)
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        vs.vector_id,
        vs.thread_id,
        vs.content,
        vs.metadata,
        vs.embedding
    FROM 
        vector_store vs
    WHERE 
        vs.thread_id = thread_id_param
        AND vs.metadata->>'namespace' = namespace_param
    ORDER BY 
        vs.created_at DESC
    LIMIT 
        limit_param;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER VOLATILE;

-- Grant execute permission to the api role
GRANT EXECUTE ON FUNCTION get_thread_vectors(TEXT, TEXT, INT) TO api;
COMMENT ON FUNCTION get_thread_vectors(TEXT, TEXT, INT) 
IS 'Get vectors from vector_store by thread ID and namespace';

--------------------------------------------------------------------------------
-- Create helper function to reload PostgREST schema cache
--------------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION postgrest_reload_schema() RETURNS TEXT AS $$
BEGIN
    NOTIFY pgrst, 'reload schema';
    RETURN 'Schema reload triggered';
END;
$$ LANGUAGE plpgsql SECURITY DEFINER VOLATILE;

GRANT EXECUTE ON FUNCTION postgrest_reload_schema() TO api;
COMMENT ON FUNCTION postgrest_reload_schema() 
IS 'Helper function to reload PostgREST schema cache';

-- Trigger a schema reload initially
SELECT postgrest_reload_schema();

