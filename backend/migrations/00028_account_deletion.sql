-- +goose Up

-- A deleted account stays as a tombstone row (email and handle replaced, no identities, no sessions) so
-- foreign keys from tank bots in past matches keep holding. banned_at is set too, so every existing
-- "not banned" guard (profiles, sessions, upload links) treats the tombstone as gone.
ALTER TABLE users ADD COLUMN deleted_at timestamptz;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
ALTER TABLE users DROP COLUMN deleted_at;
