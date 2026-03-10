-- usage_snapshots: daily rollup per org+project for analytics
CREATE TABLE IF NOT EXISTS usage_snapshots (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    project_id          UUID        REFERENCES projects(id) ON DELETE CASCADE,
    date                DATE        NOT NULL,
    api_calls           BIGINT      NOT NULL DEFAULT 0,
    chunks_created      BIGINT      NOT NULL DEFAULT 0,
    chunks_read         BIGINT      NOT NULL DEFAULT 0,
    versions_created    BIGINT      NOT NULL DEFAULT 0,
    reviews_submitted   BIGINT      NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, project_id, date)
);

CREATE INDEX IF NOT EXISTS usage_snapshots_org_date_idx
    ON usage_snapshots (org_id, date DESC);
