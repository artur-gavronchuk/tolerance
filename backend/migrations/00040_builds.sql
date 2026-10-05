-- +goose Up

-- Build challenges (internal/builds): one entry per person per challenge, re-uploads replace it in place and
-- keep its votes. The challenge catalog itself is embedded in the binary; challenge is its slug.
CREATE TABLE build_entries (
    id             text PRIMARY KEY,
    challenge      text NOT NULL,
    user_id        text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    zip            bytea NOT NULL,
    made_with      text NOT NULL DEFAULT '',
    status         text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'infra_error')),
    score          int,
    checks         jsonb NOT NULL DEFAULT '{}'::jsonb,
    shot           bytea,
    log_tail       text NOT NULL DEFAULT '',
    failure_reason text,
    uploads        int NOT NULL DEFAULT 1,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    finished_at    timestamptz,
    UNIQUE (challenge, user_id)
);
CREATE INDEX build_entries_open_idx ON build_entries (updated_at) WHERE status IN ('queued', 'running');

CREATE TABLE build_votes (
    entry_id   text NOT NULL REFERENCES build_entries (id) ON DELETE CASCADE,
    user_id    text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (entry_id, user_id)
);
CREATE INDEX build_votes_user_idx ON build_votes (user_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON build_entries, build_votes TO arena_app;

-- +goose Down
DROP TABLE build_votes;
DROP TABLE build_entries;
