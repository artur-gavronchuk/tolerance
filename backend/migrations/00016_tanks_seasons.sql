-- +goose Up

-- Tanks seasons and tournaments. A season is a calendar month (UTC); ladder ratings are per season and
-- game_bots.mu/sigma stay as the lifetime rating. Seasons are created and archived lazily by the games worker.
CREATE TABLE tanks_seasons (
    id text PRIMARY KEY,                    -- '2026-10'
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    finalized_at timestamptz
);

CREATE TABLE tanks_season_ratings (
    season_id text NOT NULL REFERENCES tanks_seasons (id),
    bot_id text NOT NULL REFERENCES game_bots (id),
    mu double precision NOT NULL DEFAULT 25,
    sigma double precision NOT NULL DEFAULT 8.333333333333334,
    matches int NOT NULL DEFAULT 0,
    wins int NOT NULL DEFAULT 0,
    PRIMARY KEY (season_id, bot_id)
);

-- The frozen final standings of an archived season.
CREATE TABLE tanks_season_standings (
    season_id text NOT NULL REFERENCES tanks_seasons (id),
    bot_id text NOT NULL REFERENCES game_bots (id),
    rank int NOT NULL,
    rating int NOT NULL,
    mu double precision NOT NULL,
    sigma double precision NOT NULL,
    matches int NOT NULL,
    wins int NOT NULL,
    bot_name text NOT NULL,
    owner text NOT NULL DEFAULT '',
    house boolean NOT NULL DEFAULT false,
    source text NOT NULL DEFAULT '',
    version int NOT NULL DEFAULT 0,
    PRIMARY KEY (season_id, bot_id)
);
CREATE INDEX tanks_season_standings_rank_idx ON tanks_season_standings (season_id, rank);

ALTER TABLE matches DROP CONSTRAINT matches_kind_check;
ALTER TABLE matches ADD CONSTRAINT matches_kind_check CHECK (kind IN ('ladder', 'check', 'tournament'));

CREATE TABLE tanks_tournaments (
    id text PRIMARY KEY,
    name text NOT NULL,
    season_id text REFERENCES tanks_seasons (id),
    status text NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'running', 'finished', 'cancelled')),
    starts_at timestamptz NOT NULL,
    started_at timestamptz,
    finished_at timestamptz,
    size int NOT NULL DEFAULT 8,            -- requested number of entrants (a power of two)
    rounds int NOT NULL DEFAULT 0,
    best_of int NOT NULL DEFAULT 3,
    champion_bot_id text REFERENCES game_bots (id),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tanks_tournaments_status_idx ON tanks_tournaments (status, starts_at);

CREATE TABLE tanks_tournament_entries (
    tournament_id text NOT NULL REFERENCES tanks_tournaments (id),
    bot_id text NOT NULL REFERENCES game_bots (id),
    version_id text NOT NULL REFERENCES bot_versions (id),
    seed int NOT NULL,
    rating int NOT NULL,
    PRIMARY KEY (tournament_id, bot_id)
);
CREATE INDEX tanks_tournament_entries_bot_idx ON tanks_tournament_entries (bot_id);

CREATE TABLE tanks_tournament_pairings (
    id text PRIMARY KEY,
    tournament_id text NOT NULL REFERENCES tanks_tournaments (id),
    round int NOT NULL,
    position int NOT NULL,
    bot_a text REFERENCES game_bots (id),
    bot_b text REFERENCES game_bots (id),
    wins_a int NOT NULL DEFAULT 0,
    wins_b int NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'finished')),
    winner_bot_id text REFERENCES game_bots (id),
    bye boolean NOT NULL DEFAULT false,
    infra_errors int NOT NULL DEFAULT 0,
    UNIQUE (tournament_id, round, position)
);

CREATE TABLE tanks_tournament_games (
    pairing_id text NOT NULL REFERENCES tanks_tournament_pairings (id),
    game int NOT NULL,
    match_id text NOT NULL REFERENCES matches (id),
    winner_bot_id text REFERENCES game_bots (id),
    PRIMARY KEY (pairing_id, game)
);
CREATE INDEX tanks_tournament_games_match_idx ON tanks_tournament_games (match_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON tanks_seasons, tanks_season_ratings, tanks_season_standings,
    tanks_tournaments, tanks_tournament_entries, tanks_tournament_pairings, tanks_tournament_games TO arena_app;

-- +goose Down
DROP TABLE tanks_tournament_games;
DROP TABLE tanks_tournament_pairings;
DROP TABLE tanks_tournament_entries;
DROP TABLE tanks_tournaments;
DELETE FROM matches WHERE kind = 'tournament';
ALTER TABLE matches DROP CONSTRAINT matches_kind_check;
ALTER TABLE matches ADD CONSTRAINT matches_kind_check CHECK (kind IN ('ladder', 'check'));
DROP TABLE tanks_season_standings;
DROP TABLE tanks_season_ratings;
DROP TABLE tanks_seasons;
