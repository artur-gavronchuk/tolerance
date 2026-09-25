-- +goose Up

ALTER TABLE proof_tasks ADD COLUMN kind text NOT NULL DEFAULT 'proof' CHECK (kind IN ('proof', 'game_bot'));
ALTER TABLE proofs ADD COLUMN kind text NOT NULL DEFAULT 'proof' CHECK (kind IN ('proof', 'game_bot')),
    ADD COLUMN repo_tar bytea,
    ADD COLUMN repo_sha256 text;

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('run_proof', 'run_match', 'check_bot'));

CREATE TABLE game_bots (
    id text PRIMARY KEY,
    game text NOT NULL CHECK (game IN ('tanks')),
    owner_user_id text REFERENCES users (id),
    agent_id text REFERENCES agents (id),
    name text NOT NULL,
    house boolean NOT NULL DEFAULT false,
    mu double precision NOT NULL DEFAULT 25,
    sigma double precision NOT NULL DEFAULT 8.333333333333334,
    matches int NOT NULL DEFAULT 0,
    wins int NOT NULL DEFAULT 0,
    active_version_id text,
    last_match_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (house OR owner_user_id IS NOT NULL)
);
CREATE UNIQUE INDEX game_bots_owner_idx ON game_bots (game, owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE UNIQUE INDEX game_bots_name_idx ON game_bots (game, lower(name));

CREATE TABLE bot_versions (
    id text PRIMARY KEY,
    bot_id text NOT NULL REFERENCES game_bots (id),
    number int NOT NULL,
    source text NOT NULL CHECK (source IN ('agent', 'upload', 'house')),
    proof_id text REFERENCES proofs (id),
    language text NOT NULL,
    entry text NOT NULL DEFAULT '',
    archive bytea NOT NULL DEFAULT ''::bytea,
    archive_sha256 text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'rejected')),
    checks jsonb NOT NULL DEFAULT '[]'::jsonb,
    check_log text NOT NULL DEFAULT '',
    check_match_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bot_id, number)
);
ALTER TABLE game_bots ADD CONSTRAINT game_bots_active_version_fk FOREIGN KEY (active_version_id) REFERENCES bot_versions (id);

CREATE TABLE matches (
    id text PRIMARY KEY,
    game text NOT NULL CHECK (game IN ('tanks')),
    kind text NOT NULL CHECK (kind IN ('ladder', 'check')),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'finished', 'infra_error')),
    seed bigint NOT NULL,
    map text NOT NULL,
    ticks int NOT NULL DEFAULT 0,
    featured boolean NOT NULL DEFAULT false,
    failure_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz
);
CREATE INDEX matches_finished_idx ON matches (game, kind, finished_at DESC) WHERE status = 'finished';
CREATE INDEX matches_open_idx ON matches (status) WHERE status IN ('queued', 'running');

CREATE TABLE match_players (
    match_id text NOT NULL REFERENCES matches (id) ON DELETE CASCADE,
    slot int NOT NULL,
    bot_id text NOT NULL REFERENCES game_bots (id),
    version_id text NOT NULL REFERENCES bot_versions (id),
    place int,
    kills int NOT NULL DEFAULT 0,
    damage int NOT NULL DEFAULT 0,
    death_tick int,
    status text NOT NULL DEFAULT '',
    answered int NOT NULL DEFAULT 0,
    asked int NOT NULL DEFAULT 0,
    noise int NOT NULL DEFAULT 0,
    mu_before double precision,
    sigma_before double precision,
    mu_after double precision,
    sigma_after double precision,
    stderr_tail text NOT NULL DEFAULT '',
    PRIMARY KEY (match_id, slot)
);
CREATE INDEX match_players_bot_idx ON match_players (bot_id);

CREATE TABLE match_replays (
    match_id text PRIMARY KEY REFERENCES matches (id) ON DELETE CASCADE,
    data bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tanks_broadcasts (
    id text PRIMARY KEY,
    match_id text NOT NULL REFERENCES matches (id),
    starts_at timestamptz NOT NULL,
    duration_ms int NOT NULL
);
CREATE INDEX tanks_broadcasts_starts_idx ON tanks_broadcasts (starts_at DESC);

GRANT SELECT, INSERT, UPDATE, DELETE ON game_bots, bot_versions, matches, match_players, match_replays, tanks_broadcasts TO arena_app;

-- +goose Down
DROP TABLE tanks_broadcasts;
DROP TABLE match_replays;
DROP TABLE match_players;
DROP TABLE matches;
ALTER TABLE game_bots DROP CONSTRAINT game_bots_active_version_fk;
DROP TABLE bot_versions;
DROP TABLE game_bots;
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('run_proof'));
ALTER TABLE proofs DROP COLUMN repo_sha256, DROP COLUMN repo_tar, DROP COLUMN kind;
ALTER TABLE proof_tasks DROP COLUMN kind;
