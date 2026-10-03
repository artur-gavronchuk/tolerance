-- +goose Up

-- Comments under a closed daily task. hidden_at/hidden_by_ban follow the moderation convention.
CREATE TABLE discussion_comments (
    id            text PRIMARY KEY,
    day           date NOT NULL,
    user_id       text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body          text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    edited_at     timestamptz,
    hidden_at     timestamptz,
    hidden_by_ban boolean NOT NULL DEFAULT false
);
CREATE INDEX discussion_comments_day_idx ON discussion_comments (day, created_at);
CREATE INDEX discussion_comments_user_idx ON discussion_comments (user_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON discussion_comments TO arena_app;

-- +goose Down
DROP TABLE discussion_comments;
