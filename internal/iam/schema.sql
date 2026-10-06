-- Identity, membership, keys, usage and audit. Applied to an empty database;
-- there is no migration from the previous JSON-membership schema.

CREATE TABLE IF NOT EXISTS bootstrap_state (
    id           INT PRIMARY KEY CHECK (id = 1),
    completed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    email           TEXT NOT NULL CHECK (email <> ''),
    name            TEXT NOT NULL DEFAULT '',
    password_hash   TEXT NOT NULL DEFAULT '',
    role            TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    session_version INT  NOT NULL DEFAULT 1,
    max_budget      NUMERIC CHECK (max_budget IS NULL OR max_budget >= 0),
    spend           NUMERIC NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower ON users (lower(email));

CREATE TABLE IF NOT EXISTS organizations (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name       TEXT NOT NULL UNIQUE CHECK (name <> ''),
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'blocked')),
    max_budget NUMERIC CHECK (max_budget IS NULL OR max_budget >= 0),
    spend      NUMERIC NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Deleting an organization that still has teams is refused (RESTRICT).
--
-- models is the team's model set and the ceiling for everything under it. An
-- empty list means "no restriction", not "no models": a team created a moment
-- ago has had nothing assigned yet, and reading that as a denial would leave it
-- unable to reach anything. A team that must reach nothing is blocked.
CREATE TABLE IF NOT EXISTS teams (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    organization_id TEXT NOT NULL REFERENCES organizations (id) ON DELETE RESTRICT,
    name            TEXT NOT NULL CHECK (name <> ''),
    description     TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'blocked')),
    models          TEXT NOT NULL DEFAULT '[]',
    max_budget      NUMERIC CHECK (max_budget IS NULL OR max_budget >= 0),
    spend           NUMERIC NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name),
    UNIQUE (id, organization_id)
);

-- An organization administrator runs one organization: its teams, their
-- projects, and the usage inside them. It is a membership rather than an
-- account role, because the same person can administer one organization and be
-- an ordinary member of another.
--
-- A platform administrator does not need a row here: the administrator role is
-- account-wide and already reaches every organization.
CREATE TABLE IF NOT EXISTS organization_members (
    organization_id TEXT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role            TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('org_admin', 'member')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);
CREATE INDEX IF NOT EXISTS organization_members_user ON organization_members (user_id);

CREATE TABLE IF NOT EXISTS team_members (
    team_id    TEXT NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('team_admin', 'member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);
CREATE INDEX IF NOT EXISTS team_members_user ON team_members (user_id);

CREATE TABLE IF NOT EXISTS projects (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    team_id    TEXT NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    name       TEXT NOT NULL CHECK (name <> ''),
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'blocked')),
    models     TEXT NOT NULL DEFAULT '[]',
    max_budget NUMERIC CHECK (max_budget IS NULL OR max_budget >= 0),
    spend      NUMERIC NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (team_id, name),
    UNIQUE (id, team_id)
);

CREATE TABLE IF NOT EXISTS api_keys (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    token_hash   TEXT NOT NULL UNIQUE,
    key_prefix   TEXT NOT NULL DEFAULT '',
    owner_type   TEXT NOT NULL CHECK (owner_type IN ('personal', 'service')),
    user_id      TEXT REFERENCES users (id) ON DELETE CASCADE,
    team_id      TEXT NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    project_id   TEXT,
    created_by   TEXT REFERENCES users (id) ON DELETE SET NULL,
    name         TEXT NOT NULL DEFAULT '',
    models       TEXT NOT NULL DEFAULT '[]',
    max_budget   NUMERIC CHECK (max_budget IS NULL OR max_budget >= 0),
    spend        NUMERIC NOT NULL DEFAULT 0,
    tpm_limit    INT CHECK (tpm_limit IS NULL OR tpm_limit >= 0),
    rpm_limit    INT CHECK (rpm_limit IS NULL OR rpm_limit >= 0),
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'blocked', 'revoked')),
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((owner_type = 'personal' AND user_id IS NOT NULL)
        OR (owner_type = 'service' AND user_id IS NULL)),
    UNIQUE (id, team_id),
    FOREIGN KEY (project_id, team_id) REFERENCES projects (id, team_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS api_keys_user ON api_keys (user_id);
CREATE INDEX IF NOT EXISTS api_keys_team ON api_keys (team_id);

-- Ownership is snapshotted at write time and has no foreign keys, so later
-- membership changes or deletions never re-attribute historical spend.
CREATE TABLE IF NOT EXISTS usage_events (
    id                BIGSERIAL PRIMARY KEY,
    request_id        TEXT NOT NULL UNIQUE,
    ts                TIMESTAMPTZ NOT NULL DEFAULT now(),
    key_id            TEXT NOT NULL DEFAULT '',
    owner_type        TEXT NOT NULL CHECK (owner_type IN ('personal', 'service', 'session')),
    user_id           TEXT NOT NULL DEFAULT '',
    team_id           TEXT NOT NULL DEFAULT '',
    project_id        TEXT NOT NULL DEFAULT '',
    organization_id   TEXT NOT NULL DEFAULT '',
    model             TEXT NOT NULL DEFAULT '',
    call_type         TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT '',
    prompt_tokens     INT NOT NULL DEFAULT 0,
    completion_tokens INT NOT NULL DEFAULT 0,
    cost              NUMERIC NOT NULL DEFAULT 0,
    duration_ms       INT NOT NULL DEFAULT 0,
    ended_at          TIMESTAMPTZ,
    ttft_ms           INT,
    cache_hit         BOOLEAN NOT NULL DEFAULT false,
    key_hash          TEXT NOT NULL DEFAULT '',
    key_alias         TEXT NOT NULL DEFAULT '',
    team_alias        TEXT NOT NULL DEFAULT '',
    provider          TEXT NOT NULL DEFAULT '',
    cached_tokens     INT,
    session_id        TEXT NOT NULL DEFAULT '',
    cache_key         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS usage_events_team_ts ON usage_events (team_id, ts);
CREATE INDEX IF NOT EXISTS usage_events_user_ts ON usage_events (user_id, ts);
CREATE INDEX IF NOT EXISTS usage_events_key_ts ON usage_events (key_id, ts);
CREATE INDEX IF NOT EXISTS usage_events_org_ts ON usage_events (organization_id, ts);

-- Tables created before these columns existed keep their rows. New installs
-- already have the columns from CREATE TABLE, so each ADD is a no-op there.
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS ended_at TIMESTAMPTZ;
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS ttft_ms INT;
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS cache_hit BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS key_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS key_alias TEXT NOT NULL DEFAULT '';
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS team_alias TEXT NOT NULL DEFAULT '';
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT '';
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS cached_tokens INT;
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS session_id TEXT NOT NULL DEFAULT '';
ALTER TABLE usage_events ADD COLUMN IF NOT EXISTS cache_key TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS usage_events_session_ts ON usage_events (session_id, ts);

CREATE TABLE IF NOT EXISTS request_logs (
    request_id    TEXT PRIMARY KEY REFERENCES usage_events (request_id) ON DELETE CASCADE,
    request_body  TEXT NOT NULL DEFAULT '',
    response_body TEXT NOT NULL DEFAULT '',
    error         TEXT NOT NULL DEFAULT '',
    proxy_request TEXT NOT NULL DEFAULT ''
);
ALTER TABLE request_logs ADD COLUMN IF NOT EXISTS proxy_request TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS usage_daily (
    day               DATE NOT NULL,
    organization_id   TEXT NOT NULL DEFAULT '',
    team_id           TEXT NOT NULL DEFAULT '',
    project_id        TEXT NOT NULL DEFAULT '',
    user_id           TEXT NOT NULL DEFAULT '',
    key_id            TEXT NOT NULL DEFAULT '',
    owner_type        TEXT NOT NULL,
    model             TEXT NOT NULL DEFAULT '',
    requests          BIGINT NOT NULL DEFAULT 0,
    prompt_tokens     BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    cost              NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (day, organization_id, team_id, project_id, user_id, key_id, owner_type, model)
);
CREATE INDEX IF NOT EXISTS usage_daily_team ON usage_daily (team_id, day);
CREATE INDEX IF NOT EXISTS usage_daily_user ON usage_daily (user_id, day);

CREATE TABLE IF NOT EXISTS audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    ts          TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_id    TEXT NOT NULL DEFAULT '',
    actor_kind  TEXT NOT NULL,
    action      TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id   TEXT NOT NULL DEFAULT '',
    team_id     TEXT NOT NULL DEFAULT '',
    detail      JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS audit_logs_ts ON audit_logs (ts);
