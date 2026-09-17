-- +goose Up

-- identity
CREATE TABLE users (
    id text PRIMARY KEY,
    oidc_issuer text NOT NULL,
    oidc_subject text NOT NULL,
    email text,
    handle text NOT NULL,
    display_name text NOT NULL,
    role text NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (oidc_issuer, oidc_subject),
    UNIQUE (handle)
);

-- agents
CREATE TABLE agents (
    id text PRIMARY KEY,
    owner_user_id text NOT NULL REFERENCES users (id),
    name text NOT NULL,
    model text NOT NULL,
    bio text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    version int NOT NULL DEFAULT 1,
    UNIQUE (owner_user_id) -- v1: one agent per user
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

-- competitions
CREATE TABLE competitions (
    id text PRIMARY KEY,
    slug text NOT NULL UNIQUE,
    title text NOT NULL,
    summary text NOT NULL,
    brief text NOT NULL,
    category text NOT NULL CHECK (category IN ('Full build', 'Bug fix', 'DB design', 'Refactor', 'Integration')),
    difficulty text NOT NULL CHECK (difficulty IN ('Easy', 'Medium', 'Hard')),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'closed')),
    points int NOT NULL CHECK (points BETWEEN 1 AND 10000),
    deadline timestamptz NOT NULL,
    match_duration_seconds int NOT NULL DEFAULT 900 CHECK (match_duration_seconds BETWEEN 300 AND 3600),
    criteria jsonb NOT NULL,
    created_by text NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    closed_at timestamptz,
    version int NOT NULL DEFAULT 1
);
CREATE INDEX competitions_status_deadline_idx ON competitions (status, deadline);

-- submissions
CREATE TABLE submissions (
    id text PRIMARY KEY,
    competition_id text NOT NULL REFERENCES competitions (id),
    agent_id text NOT NULL REFERENCES agents (id),
    source text NOT NULL CHECK (source IN ('manual', 'match')),
    match_id text,
    artifact text NOT NULL CHECK (artifact IN ('app', 'site', 'pr', 'schema')),
    summary text NOT NULL,
    preview_url text,
    repo_url text,
    preview_kind text CHECK (preview_kind IN ('diff', 'text')),
    preview_body text,
    submitted_at timestamptz NOT NULL DEFAULT now(),
    score_status text NOT NULL DEFAULT 'pending' CHECK (score_status IN ('pending', 'judging', 'scored', 'failed')),
    judgment_id text,
    scores jsonb,
    total int CHECK (total BETWEEN 0 AND 100),
    points_awarded int,
    judged_at timestamptz,
    version int NOT NULL DEFAULT 1,
    UNIQUE (competition_id, agent_id)
);
CREATE INDEX submissions_agent_idx ON submissions (agent_id, submitted_at DESC);
CREATE INDEX submissions_competition_rank_idx ON submissions (competition_id, total DESC, submitted_at ASC);

-- judging
CREATE TABLE judgments (
    id text PRIMARY KEY,
    submission_id text NOT NULL REFERENCES submissions (id),
    kind text NOT NULL CHECK (kind IN ('llm', 'human')),
    status text NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
    scores jsonb,
    total int,
    overall text,
    model text,
    prompt_version text,
    judge_user_id text REFERENCES users (id),
    usage jsonb,
    error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);
CREATE INDEX judgments_submission_idx ON judgments (submission_id, created_at DESC);
ALTER TABLE submissions ADD FOREIGN KEY (judgment_id) REFERENCES judgments (id);

-- arena
CREATE TABLE matches (
    id text PRIMARY KEY,
    competition_id text NOT NULL REFERENCES competitions (id),
    left_agent_id text NOT NULL REFERENCES agents (id),
    right_agent_id text NOT NULL REFERENCES agents (id),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'judging', 'finished', 'cancelled')),
    queue_position int,
    total_seconds int NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    submit_deadline_at timestamptz,
    judging_at timestamptz,
    finished_at timestamptz,
    left_progress int NOT NULL DEFAULT 0,
    left_phase int NOT NULL DEFAULT 0,
    left_submission_id text REFERENCES submissions (id),
    right_progress int NOT NULL DEFAULT 0,
    right_phase int NOT NULL DEFAULT 0,
    right_submission_id text REFERENCES submissions (id),
    winner_agent_id text REFERENCES agents (id),
    outcome text CHECK (outcome IN ('left', 'right', 'forfeit_left', 'forfeit_right', 'double_forfeit', 'cancelled')),
    cancel_reason text,
    version int NOT NULL DEFAULT 1,
    CHECK (left_agent_id <> right_agent_id)
);
CREATE INDEX matches_active_idx ON matches (state) WHERE state IN ('queued', 'running', 'judging');
CREATE UNIQUE INDEX matches_one_active_per_left ON matches (left_agent_id) WHERE state IN ('queued', 'running', 'judging');
CREATE UNIQUE INDEX matches_one_active_per_right ON matches (right_agent_id) WHERE state IN ('queued', 'running', 'judging');
ALTER TABLE submissions ADD FOREIGN KEY (match_id) REFERENCES matches (id);

CREATE TABLE match_events (
    id bigserial PRIMARY KEY,
    match_id text NOT NULL REFERENCES matches (id),
    side text CHECK (side IN ('left', 'right')),
    kind text NOT NULL CHECK (kind IN ('queued', 'started', 'progress', 'log', 'submitted', 'judging', 'finished', 'cancelled')),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX match_events_match_idx ON match_events (match_id, id);

CREATE TABLE arena_queue (
    agent_id text PRIMARY KEY REFERENCES agents (id),
    enqueued_at timestamptz NOT NULL DEFAULT now(),
    last_heartbeat_at timestamptz NOT NULL DEFAULT now()
);

-- standings
CREATE TABLE agent_badges (
    agent_id text NOT NULL REFERENCES agents (id),
    code text NOT NULL,
    awarded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, code)
);

-- platform
CREATE TABLE jobs (
    id text PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('judge_submission', 'recompute_badges')),
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

CREATE TABLE idempotency_records (
    actor_id text NOT NULL,
    endpoint text NOT NULL,
    key text NOT NULL,
    payload_digest text NOT NULL,
    status int NOT NULL,
    body jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (actor_id, endpoint, key)
);

-- views
CREATE VIEW competition_rankings AS
SELECT s.id AS submission_id, s.competition_id, s.agent_id, s.total, s.submitted_at,
       row_number() OVER (PARTITION BY s.competition_id ORDER BY s.total DESC, s.submitted_at ASC) AS rank,
       count(*) OVER (PARTITION BY s.competition_id) AS rank_of
FROM submissions s
WHERE s.score_status = 'scored';

CREATE VIEW agent_standings AS
WITH scored AS (
    SELECT agent_id,
           sum(points_awarded)::int AS points,
           count(*)::int AS submissions,
           round(avg(total))::int AS avg
    FROM submissions WHERE score_status = 'scored' GROUP BY agent_id
), comp_wins AS (
    SELECT r.agent_id, count(*)::int AS competition_wins
    FROM competition_rankings r
    JOIN competitions c ON c.id = r.competition_id
    WHERE r.rank = 1 AND c.status = 'closed'
      AND NOT EXISTS (SELECT 1 FROM submissions p
                      WHERE p.competition_id = c.id AND p.score_status IN ('pending', 'judging'))
    GROUP BY r.agent_id
), match_wins AS (
    SELECT winner_agent_id AS agent_id, count(*)::int AS match_wins
    FROM matches WHERE state = 'finished' AND winner_agent_id IS NOT NULL GROUP BY winner_agent_id
), base AS (
    SELECT a.id AS agent_id, a.name, u.handle AS author,
           coalesce(s.points, 0) AS points,
           coalesce(s.submissions, 0) AS submissions,
           s.avg,
           coalesce(cw.competition_wins, 0) AS competition_wins,
           coalesce(mw.match_wins, 0) AS match_wins,
           coalesce(cw.competition_wins, 0) + coalesce(mw.match_wins, 0) AS wins
    FROM agents a
    JOIN users u ON u.id = a.owner_user_id
    LEFT JOIN scored s ON s.agent_id = a.id
    LEFT JOIN comp_wins cw ON cw.agent_id = a.id
    LEFT JOIN match_wins mw ON mw.agent_id = a.id
), ranked AS (
    SELECT agent_id,
           row_number() OVER (ORDER BY points DESC, coalesce(avg, 0) DESC, lower(name) ASC) AS rank
    FROM base WHERE submissions > 0 OR wins > 0
)
SELECT b.*, r.rank
FROM base b LEFT JOIN ranked r ON r.agent_id = b.agent_id;

-- triggers
-- +goose StatementBegin
CREATE FUNCTION competitions_immutable_after_publish() RETURNS trigger AS $$
BEGIN
    IF OLD.published_at IS NOT NULL AND (
        NEW.title IS DISTINCT FROM OLD.title OR NEW.summary IS DISTINCT FROM OLD.summary OR
        NEW.brief IS DISTINCT FROM OLD.brief OR NEW.category IS DISTINCT FROM OLD.category OR
        NEW.difficulty IS DISTINCT FROM OLD.difficulty OR NEW.points IS DISTINCT FROM OLD.points OR
        NEW.deadline IS DISTINCT FROM OLD.deadline OR
        NEW.match_duration_seconds IS DISTINCT FROM OLD.match_duration_seconds OR
        NEW.criteria IS DISTINCT FROM OLD.criteria OR NEW.slug IS DISTINCT FROM OLD.slug) THEN
        RAISE EXCEPTION 'competition % is immutable after publish', OLD.id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER competitions_immutable BEFORE UPDATE ON competitions
    FOR EACH ROW EXECUTE FUNCTION competitions_immutable_after_publish();

-- +goose StatementBegin
CREATE FUNCTION match_events_notify() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('arena_events', NEW.id::text);
    RETURN NEW;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER match_events_notify AFTER INSERT ON match_events
    FOR EACH ROW EXECUTE FUNCTION match_events_notify();

-- grants
GRANT USAGE ON SCHEMA public TO arena_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO arena_app;
GRANT SELECT ON competition_rankings, agent_standings TO arena_app;

-- +goose Down
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM arena_app;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM arena_app;
REVOKE USAGE ON SCHEMA public FROM arena_app;

DROP TRIGGER match_events_notify ON match_events;
DROP FUNCTION match_events_notify();
DROP TRIGGER competitions_immutable ON competitions;
DROP FUNCTION competitions_immutable_after_publish();

DROP VIEW agent_standings;
DROP VIEW competition_rankings;

DROP TABLE idempotency_records;
DROP TABLE audit_events;
DROP TABLE jobs;
DROP TABLE agent_badges;
DROP TABLE arena_queue;
DROP TABLE match_events;
ALTER TABLE submissions DROP CONSTRAINT submissions_match_id_fkey;
DROP TABLE matches;
ALTER TABLE submissions DROP CONSTRAINT submissions_judgment_id_fkey;
DROP TABLE judgments;
DROP TABLE submissions;
DROP TABLE competitions;
DROP TABLE api_keys;
DROP TABLE agents;
DROP TABLE users;
