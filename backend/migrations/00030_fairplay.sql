-- +goose Up

-- Fair play signals. Nothing here changes a verdict or punishes anyone: flags are for humans (/admin).
-- IPs and device ids are stored only as HMAC hashes under a server secret.

-- First time a signed-in user fetched a task's repo.
CREATE TABLE fairplay_downloads (
    user_id  text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind     text NOT NULL CHECK (kind IN ('task')),
    slug     text NOT NULL,
    first_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind, slug)
);

-- Which hashed IPs / devices a user has been seen on (at downloads and uploads).
CREATE TABLE fairplay_seen (
    user_id  text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind     text NOT NULL CHECK (kind IN ('ip', 'device')),
    hash     text NOT NULL,
    first_at timestamptz NOT NULL DEFAULT now(),
    last_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind, hash)
);
CREATE INDEX fairplay_seen_hash_idx ON fairplay_seen (kind, hash);

-- One row per upload: the metrics the flags are derived from.
CREATE TABLE fairplay_uploads (
    subject_kind  text NOT NULL CHECK (subject_kind IN ('submission')),
    subject_id    text NOT NULL,
    user_id       text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    task_slug     text NOT NULL,
    day           date,
    ip_hash       text NOT NULL DEFAULT '',
    device_hash   text NOT NULL DEFAULT '',
    solve_seconds integer, -- first download to upload; null when no download was seen
    sketch        bigint[] NOT NULL DEFAULT '{}', -- bottom-k token-shingle hashes of the normalized solution
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subject_kind, subject_id)
);
CREATE INDEX fairplay_uploads_user_idx ON fairplay_uploads (user_id, created_at DESC);
CREATE INDEX fairplay_uploads_task_idx ON fairplay_uploads (task_slug, day);

CREATE TABLE fairplay_flags (
    id           text PRIMARY KEY,
    subject_kind text NOT NULL CHECK (subject_kind IN ('submission')),
    subject_id   text NOT NULL,
    user_id      text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    signal       text NOT NULL CHECK (signal IN ('fast_solve', 'burst', 'shared_device', 'shared_ip', 'near_duplicate')),
    score        real NOT NULL DEFAULT 0,
    detail       jsonb NOT NULL DEFAULT '{}',
    status       text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'dismissed', 'actioned')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    reviewed_by  text,
    reviewed_at  timestamptz,
    UNIQUE (subject_kind, subject_id, signal)
);
CREATE INDEX fairplay_flags_open_idx ON fairplay_flags (created_at DESC) WHERE status = 'open';

CREATE TABLE fairplay_reports (
    id          text PRIMARY KEY,
    reporter_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    target_kind text NOT NULL CHECK (target_kind IN ('user')),
    target_id   text NOT NULL,
    reason      text NOT NULL CHECK (reason IN ('cheating', 'copied', 'multi_account', 'offensive', 'other')),
    details     text NOT NULL DEFAULT '',
    status      text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'dismissed', 'actioned')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    reviewed_by text,
    reviewed_at timestamptz
);
CREATE INDEX fairplay_reports_open_idx ON fairplay_reports (created_at DESC) WHERE status = 'open';
CREATE UNIQUE INDEX fairplay_reports_once_idx ON fairplay_reports (reporter_id, target_kind, target_id) WHERE status = 'open';

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
DROP TABLE fairplay_reports;
DROP TABLE fairplay_flags;
DROP TABLE fairplay_uploads;
DROP TABLE fairplay_seen;
DROP TABLE fairplay_downloads;
