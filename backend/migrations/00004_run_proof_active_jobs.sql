-- +goose Up

-- ExpireStale checks, per candidate proof, whether a run_proof job for it is
-- still queued or leased. Without an index that is a sequential scan of the
-- whole jobs table per candidate; under launch load the table accumulates
-- many done/failed rows, so this partial index (only active run_proof jobs,
-- which are always few) keeps that check cheap regardless of history size.
CREATE INDEX jobs_run_proof_active_idx ON jobs ((payload ->> 'proof_id'))
    WHERE kind = 'run_proof' AND state IN ('queued', 'leased');

-- +goose Down
DROP INDEX jobs_run_proof_active_idx;
