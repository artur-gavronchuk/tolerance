-- +goose Up

-- The platform no longer connects to users' agents. Everything that existed for that goes: agents, API
-- keys, presence, proofs and the proof/skill/qualification/challenge catalogs and results. Users, identities,
-- sessions, audit events, jobs and the tanks tables stay.
ALTER TABLE game_bots DROP COLUMN agent_id;
ALTER TABLE bot_versions DROP COLUMN proof_id;

DROP TABLE IF EXISTS challenge_entries CASCADE;
DROP TABLE IF EXISTS challenges CASCADE;
DROP TABLE IF EXISTS skill_ratings CASCADE;
DROP TABLE IF EXISTS qualification_runs CASCADE;
DROP TABLE IF EXISTS skill_task_exposures CASCADE;
DROP TABLE IF EXISTS skill_tasks CASCADE;
DROP TABLE IF EXISTS skills CASCADE;
DROP TABLE IF EXISTS proofs CASCADE;
DROP TABLE IF EXISTS proof_tasks CASCADE;
DROP TABLE IF EXISTS agent_presence CASCADE;
DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS agent_versions CASCADE;
DROP TABLE IF EXISTS agents CASCADE;

DROP INDEX IF EXISTS jobs_run_proof_active_idx;
DELETE FROM jobs WHERE kind = 'run_proof';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('run_match', 'check_bot', 'run_submission'));

-- Public handle shown on leaderboards: the GitHub login when there is one, else the email's local part;
-- "-2", "-3", ... on collision (case-insensitive).
ALTER TABLE users ADD COLUMN handle text;
-- +goose StatementBegin
DO $$
DECLARE
    u record;
    base text;
    cand text;
    n int;
BEGIN
    FOR u IN
        SELECT id, email, (SELECT i.login FROM user_identities i WHERE i.user_id = users.id AND i.login <> '' ORDER BY i.created_at LIMIT 1) AS login
        FROM users ORDER BY created_at, id
    LOOP
        base := btrim(regexp_replace(coalesce(u.login, split_part(u.email, '@', 1)), '[^A-Za-z0-9_-]+', '-', 'g'), '-_');
        base := btrim(left(base, 28), '-_');
        IF base = '' THEN
            base := 'user';
        END IF;
        cand := base;
        n := 1;
        WHILE EXISTS (SELECT 1 FROM users WHERE lower(handle) = lower(cand)) LOOP
            n := n + 1;
            cand := base || '-' || n;
        END LOOP;
        UPDATE users SET handle = cand WHERE id = u.id;
    END LOOP;
END $$;
-- +goose StatementEnd
ALTER TABLE users ALTER COLUMN handle SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_handle_key UNIQUE (handle);

CREATE TABLE tasks (
    slug text PRIMARY KEY,
    title text NOT NULL,
    language text NOT NULL CHECK (language IN ('go', 'python')),
    difficulty int NOT NULL CHECK (difficulty BETWEEN 1 AND 3),
    task_md text NOT NULL,
    repo_tar bytea NOT NULL,
    hidden_tar bytea NOT NULL,
    image text NOT NULL,
    run_cmd text NOT NULL,
    sandbox_timeout_s int NOT NULL,
    hidden_tests int NOT NULL CHECK (hidden_tests > 0),
    active boolean NOT NULL DEFAULT true,
    synced_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE daily_tasks (
    day date PRIMARY KEY,
    task_slug text NOT NULL REFERENCES tasks (slug),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX daily_tasks_task_idx ON daily_tasks (task_slug, day DESC);

CREATE TABLE submissions (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users (id),
    task_slug text NOT NULL REFERENCES tasks (slug),
    day date,
    diff text NOT NULL,
    made_with text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'passed', 'failed', 'infra_error')),
    passed_tests int NOT NULL DEFAULT 0,
    total_tests int NOT NULL DEFAULT 0,
    failure_reason text,
    tests jsonb NOT NULL DEFAULT '[]'::jsonb,
    log_tail text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE INDEX submissions_day_user_idx ON submissions (day, user_id);
CREATE INDEX submissions_user_created_idx ON submissions (user_id, created_at DESC);
CREATE INDEX submissions_open_idx ON submissions (created_at) WHERE status IN ('queued', 'running');

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
SELECT 1;
