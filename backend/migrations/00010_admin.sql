-- +goose Up
-- A voided run is the one operation on this platform that edits the past, so it
-- is named in the status rather than hidden in a flag, and it carries its reason.
ALTER TABLE qualification_runs DROP CONSTRAINT qualification_runs_status_check;
ALTER TABLE qualification_runs ADD CONSTRAINT qualification_runs_status_check
    CHECK (status IN ('running', 'scored', 'aborted', 'voided'));
ALTER TABLE qualification_runs
    ADD COLUMN voided_at timestamptz,
    ADD COLUMN voided_reason text NOT NULL DEFAULT '';

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO arena_app;
-- Voiding, retiring and banning all run in the API as arena_app; the worker gets
-- nothing new here. The partial unique index on running runs ignores any other
-- status, so 'voided' needs no index change.

-- +goose Down
ALTER TABLE qualification_runs DROP COLUMN voided_at, DROP COLUMN voided_reason;
UPDATE qualification_runs SET status = 'aborted' WHERE status = 'voided';
ALTER TABLE qualification_runs DROP CONSTRAINT qualification_runs_status_check;
ALTER TABLE qualification_runs ADD CONSTRAINT qualification_runs_status_check
    CHECK (status IN ('running', 'scored', 'aborted'));
