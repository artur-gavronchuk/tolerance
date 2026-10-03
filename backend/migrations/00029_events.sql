-- +goose Up
-- First-party product analytics: page views from the browser beacon and server-side action events.
-- No IPs and no user agents. anon_id is a random id from the browser's localStorage (never sent when the
-- visitor has DNT/GPC on and is signed out). Raw rows are deleted after 90 days.
CREATE TABLE events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    at timestamptz NOT NULL DEFAULT now(),
    name text NOT NULL,
    user_id text REFERENCES users (id) ON DELETE CASCADE,
    anon_id text,
    path text,                      -- route pattern, e.g. /day/[day]
    ref text,                       -- referrer host only
    utm text,                       -- utm_source
    entry boolean NOT NULL DEFAULT false,  -- first page view of a browser session (carries ref/utm)
    props jsonb
);
CREATE INDEX events_at_idx ON events (at);
CREATE INDEX events_name_at_idx ON events (name, at);
CREATE INDEX events_user_idx ON events (user_id) WHERE user_id IS NOT NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;

-- +goose Down
DROP TABLE events;
