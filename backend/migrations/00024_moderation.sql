-- +goose Up

-- Moderation flags. Banning a user cascades hidden_at (hidden_by_ban = true) onto their submissions, product
-- entries and tank bot; unbanning clears only those. Every action is also written to audit_events.
ALTER TABLE users ADD COLUMN banned_at timestamptz;
ALTER TABLE submissions ADD COLUMN hidden_at timestamptz, ADD COLUMN hidden_by_ban boolean NOT NULL DEFAULT false;
ALTER TABLE product_entries ADD COLUMN hidden_at timestamptz, ADD COLUMN hidden_by_ban boolean NOT NULL DEFAULT false;
ALTER TABLE game_bots ADD COLUMN hidden_at timestamptz, ADD COLUMN hidden_by_ban boolean NOT NULL DEFAULT false;
CREATE INDEX audit_events_moderation_idx ON audit_events (at DESC) WHERE action LIKE 'moderation.%';

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
DROP INDEX audit_events_moderation_idx;
ALTER TABLE game_bots DROP COLUMN hidden_at, DROP COLUMN hidden_by_ban;
ALTER TABLE product_entries DROP COLUMN hidden_at, DROP COLUMN hidden_by_ban;
ALTER TABLE submissions DROP COLUMN hidden_at, DROP COLUMN hidden_by_ban;
ALTER TABLE users DROP COLUMN banned_at;
