-- +goose Up

-- Product tasks: a weekly challenge (task, deadline, a few attempts). Entries are scored by scenarios in the
-- sandbox; after the deadline they are published and opened for voting.
CREATE TABLE product_tasks (
    slug text PRIMARY KEY,
    title text NOT NULL,
    summary text NOT NULL DEFAULT '',
    kind text NOT NULL CHECK (kind IN ('cli')),
    task_md text NOT NULL,
    image text NOT NULL,
    command text NOT NULL,
    timeout_s int NOT NULL,
    scenarios jsonb NOT NULL,
    opens_at timestamptz NOT NULL DEFAULT now(),
    deadline timestamptz NOT NULL,
    active boolean NOT NULL DEFAULT true,
    synced_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE product_entries (
    id text PRIMARY KEY,
    task_slug text NOT NULL REFERENCES product_tasks (slug),
    user_id text NOT NULL REFERENCES users (id),
    zip bytea NOT NULL,
    made_with text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'infra_error')),
    passed int NOT NULL DEFAULT 0,
    total int NOT NULL DEFAULT 0,
    failure_reason text,
    results jsonb NOT NULL DEFAULT '[]'::jsonb,
    log_tail text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE INDEX product_entries_task_user_idx ON product_entries (task_slug, user_id, created_at DESC);
CREATE INDEX product_entries_open_idx ON product_entries (created_at) WHERE status IN ('queued', 'running');

CREATE TABLE product_votes (
    entry_id text NOT NULL REFERENCES product_entries (id) ON DELETE CASCADE,
    user_id text NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (entry_id, user_id)
);

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('run_match', 'check_bot', 'run_submission', 'run_product'));

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
SELECT 1;
