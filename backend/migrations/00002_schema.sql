-- +goose Up

CREATE TABLE users (
    id text PRIMARY KEY,
    email text NOT NULL UNIQUE,
    role text NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- One row per external account. subject is the provider's stable user id;
-- 'dev' identities exist only where ARENA_DEV_LOGIN is on.
CREATE TABLE user_identities (
    provider text NOT NULL CHECK (provider IN ('github', 'google', 'dev')),
    subject text NOT NULL,
    user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email text NOT NULL,
    login text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject)
);
CREATE INDEX user_identities_user_idx ON user_identities (user_id);

-- id is the SHA-256 hex of the cookie token; the token itself is never stored.
CREATE TABLE sessions (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE agents (
    id text PRIMARY KEY,
    owner_user_id text NOT NULL REFERENCES users (id),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    version int NOT NULL DEFAULT 1,
    UNIQUE (owner_user_id)
);
CREATE UNIQUE INDEX agents_name_ci_idx ON agents (lower(name));

CREATE TABLE api_keys (
    id text PRIMARY KEY,
    agent_id text NOT NULL REFERENCES agents (id),
    prefix text NOT NULL,
    key_hash text NOT NULL UNIQUE,
    name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at timestamptz
);
CREATE INDEX api_keys_agent_idx ON api_keys (agent_id) WHERE revoked_at IS NULL;

CREATE TABLE agent_presence (
    agent_id text PRIMARY KEY REFERENCES agents (id),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    connector_version text NOT NULL DEFAULT '',
    hostname text NOT NULL DEFAULT ''
);

CREATE TABLE proof_tasks (
    slug text PRIMARY KEY,
    title text NOT NULL,
    language text NOT NULL,
    image text NOT NULL,
    run_cmd text NOT NULL,
    agent_timeout_s int NOT NULL,
    sandbox_timeout_s int NOT NULL,
    visible_tests int NOT NULL,
    hidden_tests int NOT NULL,
    task_md text NOT NULL,
    repo_tar bytea NOT NULL,
    hidden_tar bytea NOT NULL,
    repo_sha256 text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE proofs (
    id text PRIMARY KEY,
    agent_id text NOT NULL REFERENCES agents (id),
    task_slug text NOT NULL REFERENCES proof_tasks (slug),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN
        ('queued', 'claimed', 'running_agent', 'diff_submitted', 'running_sandbox',
         'passed', 'failed', 'infra_error', 'expired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    claimed_at timestamptz,
    diff_submitted_at timestamptz,
    finished_at timestamptz,
    diff text NOT NULL DEFAULT '',
    agent_log_tail text NOT NULL DEFAULT '',
    agent_duration_ms int,
    agent_exit_code int,
    sandbox_result jsonb,
    failure_reason text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX proofs_one_open_idx ON proofs (agent_id)
    WHERE status IN ('queued', 'claimed', 'running_agent', 'diff_submitted', 'running_sandbox');
CREATE INDEX proofs_agent_created_idx ON proofs (agent_id, created_at DESC);

CREATE TABLE jobs (
    id text PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('run_proof')),
    dedupe_key text UNIQUE,
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'leased', 'done', 'failed')),
    run_after timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_until timestamptz,
    attempts int NOT NULL DEFAULT 0,
    max_attempts int NOT NULL DEFAULT 3,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_claim_idx ON jobs (state, run_after);

CREATE TABLE audit_events (
    id text PRIMARY KEY,
    actor_id text NOT NULL,
    actor_kind text NOT NULL CHECK (actor_kind IN ('user', 'agent', 'system')),
    action text NOT NULL,
    aggregate_kind text NOT NULL,
    aggregate_id text NOT NULL,
    before_version int,
    after_version int,
    reason text,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    request_id text,
    at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_aggregate_idx ON audit_events (aggregate_kind, aggregate_id, at DESC);

GRANT USAGE ON SCHEMA public TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
DROP TABLE audit_events;
DROP TABLE jobs;
DROP TABLE proofs;
DROP TABLE proof_tasks;
DROP TABLE agent_presence;
DROP TABLE api_keys;
DROP TABLE agents;
DROP TABLE sessions;
DROP TABLE IF EXISTS user_identities;
DROP TABLE users;
