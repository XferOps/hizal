DROP INDEX IF EXISTS context_chunks_query_key_project_idx;
DROP INDEX IF EXISTS context_chunks_fts_idx;
DROP INDEX IF EXISTS context_chunks_embedding_hnsw_idx;

DROP TABLE IF EXISTS context_reviews;
DROP TABLE IF EXISTS context_versions;
DROP TABLE IF EXISTS context_chunks;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS project_memberships;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS org_memberships;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS orgs;

DROP EXTENSION IF EXISTS vector;
DROP EXTENSION IF EXISTS "pgcrypto";
