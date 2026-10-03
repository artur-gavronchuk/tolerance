-- +goose Up

-- House agents: well-known coding agents the platform itself runs on every daily task (internal/house).
-- Their users are synced from ARENA_HOUSE_AGENTS at API start; they have no identity and cannot sign in.
-- house_name is the display name ("Claude Code · Opus").
ALTER TABLE users ADD COLUMN house boolean NOT NULL DEFAULT false, ADD COLUMN house_name text NOT NULL DEFAULT '';
CREATE INDEX users_house_idx ON users (handle) WHERE house;

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('run_match', 'check_bot', 'run_submission', 'run_product', 'run_house_agent'));

-- +goose Down
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('run_match', 'check_bot', 'run_submission', 'run_product'));
DROP INDEX users_house_idx;
ALTER TABLE users DROP COLUMN house, DROP COLUMN house_name;
