-- MEMORY chunks auto-inject the 2 most recent per agent on session start.
-- Uses the "latest" predicate (added in PR #108) to cap per-rule.
UPDATE chunk_types
SET default_inject_audience = '{"rules":[{"all":true,"latest":2}]}'::jsonb
WHERE slug = 'MEMORY' AND org_id IS NULL;

-- Backfill existing MEMORY chunks so old memories are also eligible for injection.
UPDATE context_chunks
SET inject_audience = '{"rules":[{"all":true,"latest":2}]}'::jsonb
WHERE chunk_type = 'MEMORY' AND inject_audience IS NULL;
