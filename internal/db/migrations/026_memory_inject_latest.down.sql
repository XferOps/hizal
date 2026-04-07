-- Revert MEMORY chunk type default to NULL (no auto-injection).
UPDATE chunk_types
SET default_inject_audience = NULL
WHERE slug = 'MEMORY' AND org_id IS NULL;

-- Revert existing MEMORY chunks to no injection.
UPDATE context_chunks
SET inject_audience = NULL
WHERE chunk_type = 'MEMORY'
  AND inject_audience = '{"rules":[{"all":true,"latest":2}]}'::jsonb;
