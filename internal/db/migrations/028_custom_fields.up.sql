-- Migration 028: Add custom_fields support to chunk_types and context_chunks
-- This enables typed, required/optional custom fields on chunk types with validation

-- Add custom_fields JSONB column to chunk_types for field definitions
ALTER TABLE chunk_types ADD COLUMN IF NOT EXISTS custom_fields JSONB NOT NULL DEFAULT '[]';

-- Add custom_fields JSONB column to context_chunks for field values
ALTER TABLE context_chunks ADD COLUMN IF NOT EXISTS custom_fields JSONB NOT NULL DEFAULT '{}';

-- Create GIN index on context_chunks.custom_fields for efficient filtering
CREATE INDEX IF NOT EXISTS idx_context_chunks_custom_fields ON context_chunks USING GIN(custom_fields);