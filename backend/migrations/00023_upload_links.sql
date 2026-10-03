-- +goose Up

-- One personal upload link per person: a secret token in the URL lets the person's coding agent download and
-- upload on their behalf (no session). Only the sha256 of the token is stored; rotating replaces the row.
CREATE TABLE upload_links (
    user_id text PRIMARY KEY REFERENCES users (id),
    token_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
DROP TABLE upload_links;
