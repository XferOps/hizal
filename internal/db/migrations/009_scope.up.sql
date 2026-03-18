-- 009_scope: Three-scope model for context chunks (PROJECT / AGENT / ORG)
-- and memory_enabled flag on agents.
--
-- Scope controls who can see a chunk:
--   PROJECT: shared with all project members (default, backward-compatible)
--   AGENT:   private to the owning agent (agent_id required, project_id optional)
--   ORG:     org-wide memory (org_id required, project_id NULL)
--
-- always_inject controls injection mode (see 010_always_inject):
--   false: retrieved on demand via search
--   true:  surfaced automatically as ambient baseline layer
--
-- memory_enabled on agents:
--   false (default): agent is knowledge-only — PROJECT + ORG scope only
--   true:            agent can read/write AGENT scope chunks

-- ── context_chunks ──────────────────────────────────────────────────────────

-- 1. Make project_id nullable.
--    Required for ORG scope (not project-specific) and cross-project AGENT
--    chunks (e.g. identity chunks that span all projects).
--    Backfill: all existing rows already have project_id set — no data loss.
ALTER TABLE context_chunks ALTER COLUMN project_id DROP NOT NULL;

-- 2. Add scope column (default PROJECT — fully backward-compatible).
ALTER TABLE context_chunks
    ADD COLUMN IF NOT EXISTS scope VARCHAR(20) NOT NULL DEFAULT 'PROJECT';

-- 3. Add agent_id FK for AGENT-scoped chunks.
ALTER TABLE context_chunks
    ADD COLUMN IF NOT EXISTS agent_id UUID REFERENCES agents(id) ON DELETE CASCADE;

-- 4. Add org_id FK for ORG-scoped chunks.
--    Note: org_id can be derived from project for PROJECT scope, but storing
--    it directly on ORG-scoped chunks avoids a join and clarifies ownership.
ALTER TABLE context_chunks
    ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES orgs(id) ON DELETE CASCADE;

-- 5. Check constraints — enforce the scope/FK relationship.
ALTER TABLE context_chunks
    ADD CONSTRAINT chk_scope_project
        CHECK (scope != 'PROJECT' OR project_id IS NOT NULL);

ALTER TABLE context_chunks
    ADD CONSTRAINT chk_scope_agent
        CHECK (scope != 'AGENT' OR agent_id IS NOT NULL);

ALTER TABLE context_chunks
    ADD CONSTRAINT chk_scope_org
        CHECK (scope != 'ORG' OR (org_id IS NOT NULL AND project_id IS NULL));

-- 6. Indexes for the new FK columns and scope-based queries.
CREATE INDEX IF NOT EXISTS context_chunks_agent_id_idx
    ON context_chunks (agent_id) WHERE agent_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS context_chunks_org_id_idx
    ON context_chunks (org_id) WHERE org_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS context_chunks_scope_project_idx
    ON context_chunks (project_id, scope) WHERE scope = 'PROJECT';

-- ── agents ───────────────────────────────────────────────────────────────────

-- 7. Add always_inject column.
--    false (default): retrieved on demand via semantic search.
--    true:  surfaced automatically as ambient baseline layer.
--    Always-inject chunks are used for identity (always_inject:true) and
--    org principles (store_principle). Regular knowledge is false.
ALTER TABLE context_chunks
    ADD COLUMN IF NOT EXISTS always_inject BOOLEAN NOT NULL DEFAULT FALSE;

-- 8. Add chunk_type column.
--    KNOWLEDGE: facts, architecture, conventions (default, most common)
--    RESEARCH:  investigation, findings (most disposable during consolidation)
--    PLAN:      planned work, approaches
--    DECISION:  made decisions, reasoning
--    Chunk type is orthogonal to scope and always_inject.
ALTER TABLE context_chunks
    ADD COLUMN IF NOT EXISTS chunk_type VARCHAR(20) NOT NULL DEFAULT 'KNOWLEDGE';

-- 9. Indexes for always_inject and chunk_type queries.
CREATE INDEX IF NOT EXISTS context_chunks_always_inject_idx
    ON context_chunks (project_id, always_inject) WHERE always_inject = TRUE;

CREATE INDEX IF NOT EXISTS context_chunks_chunk_type_idx
    ON context_chunks (project_id, chunk_type);

-- ── agents ───────────────────────────────────────────────────────────────────

-- 10. Add memory_enabled to agents.
--     false (default): knowledge-only agent — PROJECT + ORG scope only.
--     true:            full behavior-driven agent — AGENT scope unlocked.
--     Enforcement: write_identity / write_memory reject if memory_enabled = false.
ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS memory_enabled BOOLEAN NOT NULL DEFAULT FALSE;
