-- Add tags array to context_chunks for public discovery filtering.
ALTER TABLE context_chunks ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX IF NOT EXISTS idx_context_chunks_tags ON context_chunks USING GIN(tags);
