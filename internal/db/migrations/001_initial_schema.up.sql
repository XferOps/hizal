-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- orgs
CREATE TABLE IF NOT EXISTS orgs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL,
    slug       VARCHAR(255) NOT NULL UNIQUE,
    tier       VARCHAR(50)  NOT NULL DEFAULT 'free',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- users
CREATE TABLE IF NOT EXISTS users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email      VARCHAR(255) NOT NULL UNIQUE,
    name       VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- org_memberships
CREATE TABLE IF NOT EXISTS org_memberships (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id     UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    role       VARCHAR(50) NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, org_id)
);

-- projects
CREATE TABLE IF NOT EXISTS projects (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID         NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name       VARCHAR(255) NOT NULL,
    slug       VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, slug)
);

-- project_memberships
CREATE TABLE IF NOT EXISTS project_memberships (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role       VARCHAR(50) NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, project_id)
);

-- api_keys
CREATE TABLE IF NOT EXISTS api_keys (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash             VARCHAR(255) NOT NULL UNIQUE,
    name                 VARCHAR(255) NOT NULL,
    scope_all_projects   BOOLEAN      NOT NULL DEFAULT FALSE,
    allowed_project_ids  UUID[]       NOT NULL DEFAULT '{}',
    permissions          JSONB        NOT NULL DEFAULT '{}',
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_used_at         TIMESTAMPTZ
);

-- context_chunks
CREATE TABLE IF NOT EXISTS context_chunks (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id        UUID         NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    query_key         VARCHAR(255) NOT NULL,
    title             VARCHAR(500) NOT NULL,
    content           JSONB        NOT NULL DEFAULT '{}',
    embedding         vector(1536),
    source_file       VARCHAR(500),
    source_lines      JSONB        NOT NULL DEFAULT '{}',
    gotchas           JSONB        NOT NULL DEFAULT '[]',
    related           JSONB        NOT NULL DEFAULT '[]',
    created_by_agent  VARCHAR(255),
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- context_versions
CREATE TABLE IF NOT EXISTS context_versions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chunk_id       UUID        NOT NULL REFERENCES context_chunks(id) ON DELETE CASCADE,
    version        INT         NOT NULL,
    content        JSONB       NOT NULL DEFAULT '{}',
    change_note    TEXT,
    compacted_from JSONB       NOT NULL DEFAULT '[]',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- context_reviews
CREATE TABLE IF NOT EXISTS context_reviews (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chunk_id         UUID         NOT NULL REFERENCES context_chunks(id) ON DELETE CASCADE,
    task             VARCHAR(500),
    usefulness       INT,
    usefulness_note  TEXT,
    correctness      INT,
    correctness_note TEXT,
    action           VARCHAR(100),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes

-- HNSW index for vector similarity search (cosine)
CREATE INDEX IF NOT EXISTS context_chunks_embedding_hnsw_idx
    ON context_chunks USING hnsw (embedding vector_cosine_ops);

-- GIN index for full-text search on title + content
CREATE INDEX IF NOT EXISTS context_chunks_fts_idx
    ON context_chunks USING gin (
        to_tsvector('english', title || ' ' || COALESCE(content->>'text', ''))
    );

-- Btree index on query_key + project_id for fast lookups
CREATE INDEX IF NOT EXISTS context_chunks_query_key_project_idx
    ON context_chunks (query_key, project_id);
