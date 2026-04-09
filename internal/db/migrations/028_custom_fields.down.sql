-- Migration 028: Rollback custom_fields columns

DROP INDEX IF EXISTS idx_context_chunks_custom_fields;

ALTER TABLE context_chunks DROP COLUMN IF EXISTS custom_fields;

ALTER TABLE chunk_types DROP COLUMN IF EXISTS custom_fields;