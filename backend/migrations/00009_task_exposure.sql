-- +goose Up

-- Exposure is counted per distinct agent, not per handout: a task re-issued to
-- the same agent after an infra_error does not widen the leak, a new owner does.
CREATE TABLE skill_task_exposures (
    task_slug text NOT NULL REFERENCES skill_tasks (slug),
    agent_id text NOT NULL REFERENCES agents (id),
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (task_slug, agent_id)
);

-- Expand-only, for canary releases: every column is defaulted or nullable, so
-- an API that predates this migration keeps working on these rows.
ALTER TABLE skill_tasks
    ADD COLUMN exposures int NOT NULL DEFAULT 0,
    ADD COLUMN first_used_at timestamptz,
    ADD COLUMN retired_at timestamptz,
    ADD COLUMN retired_reason text NOT NULL DEFAULT '',
    ADD COLUMN runs int NOT NULL DEFAULT 0,
    ADD COLUMN sum_score numeric(10,4) NOT NULL DEFAULT 0,
    ADD COLUMN challenge_only boolean NOT NULL DEFAULT false;

CREATE INDEX skill_tasks_pool_idx ON skill_tasks (skill_slug)
    WHERE active AND retired_at IS NULL AND NOT challenge_only;

ALTER TABLE agents
    ADD COLUMN public boolean NOT NULL DEFAULT true,
    ADD COLUMN banned_at timestamptz,
    ADD COLUMN banned_reason text NOT NULL DEFAULT '';

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;
-- Least-privilege grants for arena_worker (same rule as 00007/00008: only the
-- verbs its own code paths need). The worker queues the next task of a
-- qualification run, which records an exposure, and accumulates observed
-- difficulty when a run is scored. Retiring by hand is the admin API, which
-- runs as arena_app; the automatic retire rides along inside the same UPDATE.
GRANT SELECT, INSERT ON skill_task_exposures TO arena_worker;
GRANT UPDATE ON skill_tasks TO arena_worker;

-- +goose Down
REVOKE ALL ON skill_task_exposures FROM arena_worker;
REVOKE UPDATE ON skill_tasks FROM arena_worker;
DROP TABLE skill_task_exposures;
DROP INDEX skill_tasks_pool_idx;
ALTER TABLE skill_tasks DROP COLUMN exposures, DROP COLUMN first_used_at, DROP COLUMN retired_at,
    DROP COLUMN retired_reason, DROP COLUMN runs, DROP COLUMN sum_score, DROP COLUMN challenge_only;
ALTER TABLE agents DROP COLUMN public, DROP COLUMN banned_at, DROP COLUMN banned_reason;
