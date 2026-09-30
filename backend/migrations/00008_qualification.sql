-- +goose Up

CREATE TABLE agent_versions (
    id text PRIMARY KEY,
    agent_id text NOT NULL REFERENCES agents (id),
    number int NOT NULL,
    model text NOT NULL DEFAULT '',
    harness text NOT NULL DEFAULT '',
    config_digest text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (agent_id, config_digest),
    UNIQUE (agent_id, number)
);
ALTER TABLE agents ADD COLUMN current_version_id text REFERENCES agent_versions (id);

CREATE TABLE skills (
    slug text PRIMARY KEY,
    title text NOT NULL,
    language text NOT NULL,
    image text NOT NULL,
    run_cmd text NOT NULL,
    description text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE skill_tasks (
    slug text PRIMARY KEY,
    skill_slug text NOT NULL REFERENCES skills (slug),
    title text NOT NULL,
    difficulty int NOT NULL CHECK (difficulty BETWEEN 1 AND 3),
    agent_timeout_s int NOT NULL,
    sandbox_timeout_s int NOT NULL,
    hidden_tests int NOT NULL,
    task_md text NOT NULL,
    repo_tar bytea NOT NULL,
    hidden_tar bytea NOT NULL,
    repo_sha256 text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX skill_tasks_skill_idx ON skill_tasks (skill_slug);

CREATE TABLE qualification_runs (
    id text PRIMARY KEY,
    agent_id text NOT NULL REFERENCES agents (id),
    version_id text NOT NULL REFERENCES agent_versions (id),
    skill_slug text NOT NULL REFERENCES skills (slug),
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'scored', 'aborted')),
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    score numeric(5,4),
    rating_before int,
    rating_after int,
    uncertainty_after int,
    task_slugs text[] NOT NULL
);
CREATE UNIQUE INDEX qualification_runs_one_open_idx ON qualification_runs (agent_id) WHERE status = 'running';
CREATE INDEX qualification_runs_agent_idx ON qualification_runs (agent_id, created_at DESC);

-- proofs.kind comes from 00003_games (proof | game_bot); its inline CHECK is proofs_kind_check.
-- Expand-only for canary releases: every new proofs column is nullable or
-- defaulted, and task_slug only loses NOT NULL; code that predates this
-- migration keeps working on kind IN ('proof', 'game_bot') rows.
ALTER TABLE proofs DROP CONSTRAINT proofs_kind_check;
ALTER TABLE proofs ADD CONSTRAINT proofs_kind_check CHECK (kind IN ('proof', 'game_bot', 'qualification'));
ALTER TABLE proofs
    ADD COLUMN qualification_run_id text REFERENCES qualification_runs (id),
    ADD COLUMN position int,
    ADD COLUMN skill_task_slug text REFERENCES skill_tasks (slug),
    ADD COLUMN retried_infra boolean NOT NULL DEFAULT false;
ALTER TABLE proofs ALTER COLUMN task_slug DROP NOT NULL;
ALTER TABLE proofs ADD CONSTRAINT proofs_task_ref CHECK (
    (kind IN ('proof', 'game_bot') AND task_slug IS NOT NULL) OR (kind = 'qualification' AND skill_task_slug IS NOT NULL AND qualification_run_id IS NOT NULL AND position BETWEEN 1 AND 3));
CREATE INDEX proofs_run_idx ON proofs (qualification_run_id, position);

CREATE TABLE skill_ratings (
    agent_id text NOT NULL REFERENCES agents (id),
    skill_slug text NOT NULL REFERENCES skills (slug),
    version_id text NOT NULL REFERENCES agent_versions (id),
    rating int NOT NULL,
    uncertainty int NOT NULL,
    runs int NOT NULL DEFAULT 0,
    sum_targets int NOT NULL DEFAULT 0,
    prior_rating int,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, skill_slug)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- Least-privilege grants for arena_worker (see 00006_worker_role.go and 00007_worker_grants.sql; same rule:
-- only the verbs its own code paths need, no ALTER DEFAULT PRIVILEGES). The worker process runs
-- proofs.Worker.RunProof (reads skill_tasks/skills for a qualification proof), qualifications.OnProofFinished
-- (after every terminal proof transition) and qualifications.SweepStalled (maintenance loop). Run creation
-- (Start), version switching (agents.version.go, the VersionListener) and the public reads run in the api
-- role as arena_app and need nothing here.
-- The next task of a run is queued from the worker (CreateQualificationProof): proofs was SELECT, UPDATE.
-- The run_proof job for it goes through jobs, which already has INSERT (00007).
GRANT INSERT ON proofs TO arena_worker;
-- The skills catalog is written only by cmd/migrate (as arena_migrate); the worker reads the task a
-- qualification proof points at (repo/hidden tarballs, timeouts, run_cmd/image via skills) and scores by
-- difficulty and skill.
GRANT SELECT ON skills, skill_tasks TO arena_worker;
-- A run is created by the API; the worker advances, scores, aborts and sweeps it, never inserts.
GRANT SELECT, UPDATE ON qualification_runs TO arena_worker;
-- Scoring reads the rating state and upserts it (INSERT ... ON CONFLICT DO UPDATE needs both verbs).
GRANT SELECT, INSERT, UPDATE ON skill_ratings TO arena_worker;
-- Agent versions are neither read nor written by worker code: a run stores its own version_id, and the
-- version listener and RatingsFor run in the api role. agents(current_version_id) is therefore not granted.

-- +goose Down
REVOKE ALL ON skill_ratings, qualification_runs, skill_tasks, skills FROM arena_worker;
REVOKE INSERT ON proofs FROM arena_worker;
DROP TABLE skill_ratings;
DELETE FROM proofs WHERE kind = 'qualification';
ALTER TABLE proofs DROP CONSTRAINT proofs_task_ref;
ALTER TABLE proofs DROP COLUMN qualification_run_id, DROP COLUMN position, DROP COLUMN skill_task_slug, DROP COLUMN retried_infra;
ALTER TABLE proofs ALTER COLUMN task_slug SET NOT NULL;
ALTER TABLE proofs DROP CONSTRAINT proofs_kind_check;
ALTER TABLE proofs ADD CONSTRAINT proofs_kind_check CHECK (kind IN ('proof', 'game_bot'));
DROP TABLE qualification_runs;
DROP TABLE skill_tasks;
DROP TABLE skills;
ALTER TABLE agents DROP COLUMN current_version_id;
DROP TABLE agent_versions;
