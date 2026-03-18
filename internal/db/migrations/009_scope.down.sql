-- 009_scope rollback

ALTER TABLE agents DROP COLUMN IF EXISTS memory_enabled;

ALTER TABLE context_chunks DROP CONSTRAINT IF EXISTS chk_scope_org;
ALTER TABLE context_chunks DROP CONSTRAINT IF EXISTS chk_scope_agent;
ALTER TABLE context_chunks DROP CONSTRAINT IF EXISTS chk_scope_project;

DROP INDEX IF EXISTS context_chunks_chunk_type_idx;
DROP INDEX IF EXISTS context_chunks_always_inject_idx;
DROP INDEX IF EXISTS context_chunks_scope_project_idx;
DROP INDEX IF EXISTS context_chunks_org_id_idx;
DROP INDEX IF EXISTS context_chunks_agent_id_idx;

ALTER TABLE context_chunks DROP COLUMN IF EXISTS chunk_type;
ALTER TABLE context_chunks DROP COLUMN IF EXISTS always_inject;
ALTER TABLE context_chunks DROP COLUMN IF EXISTS org_id;
ALTER TABLE context_chunks DROP COLUMN IF EXISTS agent_id;
ALTER TABLE context_chunks DROP COLUMN IF EXISTS scope;

ALTER TABLE context_chunks ALTER COLUMN project_id SET NOT NULL;
