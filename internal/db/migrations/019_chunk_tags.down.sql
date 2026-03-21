DROP INDEX IF EXISTS idx_context_chunks_tags;
ALTER TABLE context_chunks DROP COLUMN IF EXISTS tags;
