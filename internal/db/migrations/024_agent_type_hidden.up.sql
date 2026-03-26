-- 024_agent_type_hidden.up.sql
ALTER TABLE agent_types ADD COLUMN hidden BOOLEAN DEFAULT false NOT NULL;