-- Ensure we can generate UUIDs
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Drop tables if they exist (optional if you only want to run once):
-- (In production, you'd use proper migrations instead of drops)
DROP TABLE IF EXISTS messages CASCADE;
DROP TABLE IF EXISTS threads CASCADE;

-- threads table
CREATE TABLE IF NOT EXISTS threads (
    thread_id    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    model        VARCHAR(255) NOT NULL,
    provider     VARCHAR(255) NOT NULL,
    tokens_spent BIGINT NOT NULL DEFAULT 0,
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- messages table
CREATE TABLE IF NOT EXISTS messages (
    message_id  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    thread_id   UUID NOT NULL,
    role        VARCHAR(50) NOT NULL,  -- user | assistant | system
    content     JSONB NOT NULL,        -- flexible JSON for text blocks
    attachments TEXT[] DEFAULT '{}',   -- array of file IDs (strings)
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_thread
      FOREIGN KEY(thread_id) 
      REFERENCES threads(thread_id) 
      ON DELETE CASCADE
);

-- Create "anonymous" role to support PostgREST
CREATE ROLE anonymous NOLOGIN;

-- Grant usage on schema to anonymous
GRANT USAGE ON SCHEMA public TO anonymous;

-- Optionally grant read-only privileges to anonymous
-- GRANT SELECT ON threads, messages TO anonymous;

-- Create a dedicated role (NOLOGIN means it can't directly log in via normal DB clients)
CREATE ROLE api NOLOGIN;

-- Grant usage on the public schema
GRANT USAGE ON SCHEMA public TO api;

-- Grant the necessary table privileges
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE threads TO api;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE messages TO api;
