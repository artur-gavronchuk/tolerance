-- +goose Up

-- On-site notifications: generated lazily on read from existing data, one row per (user, natural key).
-- type + params are rendered by the frontend (en/ru).
CREATE TABLE notifications (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users (id),
    key text NOT NULL,
    type text NOT NULL,
    params jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at timestamptz,
    UNIQUE (user_id, key)
);
CREATE INDEX notifications_user_created_idx ON notifications (user_id, created_at DESC);

-- The best season rank a person's bot held since their last "dropped" notice (reset with the season).
CREATE TABLE notify_state (
    user_id text PRIMARY KEY REFERENCES users (id),
    season_id text NOT NULL,
    bot_rank int NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
DROP TABLE notify_state;
DROP TABLE notifications;
