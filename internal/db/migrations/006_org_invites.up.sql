-- WNW-29: Email invite flow for org membership
CREATE TABLE org_invites (
  id           TEXT        NOT NULL PRIMARY KEY,
  org_id       TEXT        NOT NULL REFERENCES orgs(id)  ON DELETE CASCADE,
  invited_by   TEXT        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  email        TEXT        NOT NULL,
  role         TEXT        NOT NULL DEFAULT 'member',
  token        TEXT        NOT NULL UNIQUE,  -- secure random token, used once
  expires_at   TIMESTAMPTZ NOT NULL,
  accepted_at  TIMESTAMPTZ,                  -- NULL = pending
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX org_invites_org_idx   ON org_invites(org_id);
CREATE INDEX org_invites_token_idx ON org_invites(token);
