-- +goose Up

-- Build challenge seasons: digest identifies an upload's content (path + sha256 of every file), so a copy of
-- another entry is refused; reference marks the platform's own reference solution (out of ranking and votes).
-- score now holds the hidden-test points (0..60); votes add up to 40 when the gallery is read.
ALTER TABLE build_entries ADD COLUMN digest text NOT NULL DEFAULT '', ADD COLUMN reference boolean NOT NULL DEFAULT false;
CREATE INDEX build_entries_digest_idx ON build_entries (challenge, digest);

-- +goose Down
DROP INDEX build_entries_digest_idx;
ALTER TABLE build_entries DROP COLUMN digest, DROP COLUMN reference;
