-- +goose Up

-- The first competition carries its published task bundle and names its check suite.
ALTER TABLE competitions ADD COLUMN task jsonb, ADD COLUMN check_suite text;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION competitions_immutable_after_publish() RETURNS trigger AS $$
BEGIN
    IF OLD.published_at IS NOT NULL AND (
        NEW.title IS DISTINCT FROM OLD.title OR NEW.summary IS DISTINCT FROM OLD.summary OR
        NEW.brief IS DISTINCT FROM OLD.brief OR NEW.category IS DISTINCT FROM OLD.category OR
        NEW.difficulty IS DISTINCT FROM OLD.difficulty OR NEW.points IS DISTINCT FROM OLD.points OR
        NEW.deadline IS DISTINCT FROM OLD.deadline OR
        NEW.match_duration_seconds IS DISTINCT FROM OLD.match_duration_seconds OR
        NEW.criteria IS DISTINCT FROM OLD.criteria OR NEW.slug IS DISTINCT FROM OLD.slug OR
        NEW.task IS DISTINCT FROM OLD.task OR NEW.check_suite IS DISTINCT FROM OLD.check_suite) THEN
        RAISE EXCEPTION 'competition % is immutable after publish', OLD.id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- attempts: one official attempt per (competition, agent), any number of practice attempts.
CREATE TABLE attempts (
    id text PRIMARY KEY,
    competition_id text NOT NULL REFERENCES competitions (id),
    agent_id text NOT NULL REFERENCES agents (id),
    kind text NOT NULL CHECK (kind IN ('official', 'practice')),
    attempt_no int NOT NULL CHECK (attempt_no >= 1),
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'submitted', 'abandoned')),
    agent_snapshot jsonb NOT NULL,
    reported_cost jsonb,
    match_id text REFERENCES matches (id),
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    voided_at timestamptz,
    void_reason text,
    UNIQUE (competition_id, agent_id, attempt_no)
);
CREATE UNIQUE INDEX attempts_one_official ON attempts (competition_id, agent_id)
    WHERE kind = 'official' AND voided_at IS NULL;
CREATE UNIQUE INDEX attempts_one_running ON attempts (competition_id, agent_id)
    WHERE status = 'running' AND voided_at IS NULL;

CREATE TABLE attempt_events (
    id bigserial PRIMARY KEY,
    attempt_id text NOT NULL REFERENCES attempts (id),
    kind text NOT NULL CHECK (kind IN ('started', 'phase', 'log', 'preview_available', 'build_finished',
                                       'submitted', 'check_started', 'check_finished', 'result', 'abandoned')),
    phase_index int CHECK (phase_index BETWEEN 0 AND 8),
    text text,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attempt_events_attempt_idx ON attempt_events (attempt_id, id);

-- submissions: many per (competition, agent) now; exactly one official.
ALTER TABLE submissions DROP CONSTRAINT submissions_competition_id_agent_id_key;
ALTER TABLE submissions DROP CONSTRAINT submissions_score_status_check;
ALTER TABLE submissions ADD CONSTRAINT submissions_score_status_check
    CHECK (score_status IN ('pending', 'judging', 'scored', 'unverifiable', 'failed'));
ALTER TABLE submissions
    ADD COLUMN attempt_id text REFERENCES attempts (id),
    ADD COLUMN attempt_kind text NOT NULL DEFAULT 'official' CHECK (attempt_kind IN ('official', 'practice')),
    ADD COLUMN attempt_no int NOT NULL DEFAULT 1,
    ADD COLUMN agent_snapshot jsonb,
    ADD COLUMN commit_sha text,
    ADD COLUMN notes text,
    ADD COLUMN reported_cost jsonb,
    ADD COLUMN verification text NOT NULL DEFAULT 'self_reported' CHECK (verification IN ('self_reported')),
    ADD COLUMN commit_link text NOT NULL DEFAULT 'not_provided'
        CHECK (commit_link IN ('not_provided', 'unverified', 'declared_match', 'mismatch')),
    ADD COLUMN check_run_id text,
    ADD COLUMN unscored_reason text;
CREATE UNIQUE INDEX submissions_one_official ON submissions (competition_id, agent_id) WHERE attempt_kind = 'official';
CREATE UNIQUE INDEX submissions_one_per_attempt ON submissions (attempt_id) WHERE attempt_id IS NOT NULL;

-- checks: a run is one execution of a suite against a submission's preview URL.
CREATE TABLE check_runs (
    id text PRIMARY KEY,
    submission_id text NOT NULL REFERENCES submissions (id),
    suite text NOT NULL,
    suite_version text NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'completed', 'unreachable', 'infra_error')),
    checker_version text,
    browser text,
    functional_score int CHECK (functional_score BETWEEN 0 AND 100),
    unreachable_reason text,
    error text,
    build_info jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz
);
CREATE INDEX check_runs_submission_idx ON check_runs (submission_id, created_at DESC);
ALTER TABLE submissions ADD FOREIGN KEY (check_run_id) REFERENCES check_runs (id);

CREATE TABLE check_results (
    id text PRIMARY KEY,
    check_run_id text NOT NULL REFERENCES check_runs (id),
    check_id text NOT NULL,
    title text NOT NULL,
    requirement text NOT NULL,
    required boolean NOT NULL,
    weight int NOT NULL,
    status text NOT NULL CHECK (status IN ('passed', 'failed', 'insufficient_data', 'infra_error')),
    expected text NOT NULL DEFAULT '',
    actual text NOT NULL DEFAULT '',
    diagnostics jsonb NOT NULL DEFAULT '{}'::jsonb,
    duration_ms int,
    override_status text CHECK (override_status IN ('passed', 'failed', 'insufficient_data')),
    override_reason text,
    override_by text REFERENCES users (id),
    override_at timestamptz,
    UNIQUE (check_run_id, check_id)
);

CREATE TABLE evidence_blobs (
    id text PRIMARY KEY,
    check_run_id text NOT NULL REFERENCES check_runs (id),
    check_id text,
    kind text NOT NULL CHECK (kind IN ('screenshot', 'page_text', 'console', 'network')),
    label text NOT NULL,
    content_type text NOT NULL CHECK (content_type IN ('image/png', 'image/jpeg', 'text/plain', 'application/json')),
    bytes bytea NOT NULL,
    size int NOT NULL CHECK (size BETWEEN 1 AND 1048576),
    sha256 text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX evidence_blobs_run_idx ON evidence_blobs (check_run_id, check_id);

ALTER TABLE judgments ADD COLUMN check_run_id text REFERENCES check_runs (id), ADD COLUMN reason text;

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('check_submission', 'judge_submission', 'recompute_badges'));

-- Rankings and standings count official, scored submissions only.
DROP VIEW agent_standings;
DROP VIEW competition_rankings;
CREATE VIEW competition_rankings AS
SELECT s.id AS submission_id, s.competition_id, s.agent_id, s.total, s.submitted_at,
       row_number() OVER (PARTITION BY s.competition_id ORDER BY s.total DESC, s.submitted_at ASC) AS rank,
       count(*) OVER (PARTITION BY s.competition_id) AS rank_of
FROM submissions s
WHERE s.score_status = 'scored' AND s.attempt_kind = 'official';

CREATE VIEW agent_standings AS
WITH scored AS (
    SELECT agent_id,
           sum(points_awarded)::int AS points,
           count(*)::int AS submissions,
           round(avg(total))::int AS avg
    FROM submissions WHERE score_status = 'scored' AND attempt_kind = 'official' GROUP BY agent_id
), comp_wins AS (
    SELECT r.agent_id, count(*)::int AS competition_wins
    FROM competition_rankings r
    JOIN competitions c ON c.id = r.competition_id
    WHERE r.rank = 1 AND c.status = 'closed'
      AND NOT EXISTS (SELECT 1 FROM submissions p
                      WHERE p.competition_id = c.id AND p.attempt_kind = 'official' AND p.score_status IN ('pending', 'judging'))
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

GRANT SELECT, INSERT, UPDATE, DELETE ON attempts, attempt_events, check_runs, check_results, evidence_blobs TO arena_app;
GRANT USAGE, SELECT ON SEQUENCE attempt_events_id_seq TO arena_app;
GRANT SELECT ON competition_rankings, agent_standings TO arena_app;

-- +goose Down
DROP VIEW agent_standings;
DROP VIEW competition_rankings;

ALTER TABLE judgments DROP COLUMN check_run_id, DROP COLUMN reason;

DELETE FROM jobs WHERE kind = 'check_submission';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('judge_submission', 'recompute_badges'));

DROP TABLE evidence_blobs;
DROP TABLE check_results;
ALTER TABLE submissions DROP CONSTRAINT submissions_check_run_id_fkey;
DROP TABLE check_runs;

-- Practice submissions cannot exist under the old one-per-agent rule.
DELETE FROM submissions WHERE attempt_kind = 'practice';
UPDATE submissions SET score_status = 'failed' WHERE score_status = 'unverifiable';
DROP INDEX submissions_one_per_attempt;
DROP INDEX submissions_one_official;
ALTER TABLE submissions
    DROP COLUMN unscored_reason, DROP COLUMN check_run_id, DROP COLUMN commit_link, DROP COLUMN verification,
    DROP COLUMN reported_cost, DROP COLUMN notes, DROP COLUMN commit_sha, DROP COLUMN agent_snapshot,
    DROP COLUMN attempt_no, DROP COLUMN attempt_kind, DROP COLUMN attempt_id;
ALTER TABLE submissions DROP CONSTRAINT submissions_score_status_check;
ALTER TABLE submissions ADD CONSTRAINT submissions_score_status_check
    CHECK (score_status IN ('pending', 'judging', 'scored', 'failed'));
ALTER TABLE submissions ADD CONSTRAINT submissions_competition_id_agent_id_key UNIQUE (competition_id, agent_id);

DROP TABLE attempt_events;
DROP TABLE attempts;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION competitions_immutable_after_publish() RETURNS trigger AS $$
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

ALTER TABLE competitions DROP COLUMN task, DROP COLUMN check_suite;

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
GRANT SELECT ON competition_rankings, agent_standings TO arena_app;
