-- +goose Up
-- A pairing whose matches keep failing for platform reasons stops as 'stalled' (no winner, bracket waits)
-- until an admin resumes it, instead of being decided by seed.
ALTER TABLE tanks_tournament_pairings DROP CONSTRAINT tanks_tournament_pairings_status_check;
ALTER TABLE tanks_tournament_pairings ADD CONSTRAINT tanks_tournament_pairings_status_check
    CHECK (status IN ('pending', 'running', 'finished', 'stalled'));

-- +goose Down
SELECT 1;
