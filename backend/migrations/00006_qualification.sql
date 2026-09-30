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

-- +goose Down
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
