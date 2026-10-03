-- +goose Up

-- The weekly product tasks are gone: tables, their queued jobs, notifications and moderation records.
-- Table grants for arena_app go with the tables; nothing else referenced them.
DROP TABLE IF EXISTS product_judgments CASCADE;
DROP TABLE IF EXISTS product_votes CASCADE;
DROP TABLE IF EXISTS product_entries CASCADE;
DROP TABLE IF EXISTS product_tasks CASCADE;

DELETE FROM jobs WHERE kind = 'run_product';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check CHECK (kind IN ('run_match', 'check_bot', 'run_submission'));

DELETE FROM notifications WHERE type LIKE 'product\_%';
DELETE FROM audit_events WHERE action LIKE 'moderation.%' AND aggregate_kind = 'entry';

-- Left over from the connector's run_proof jobs; 00012 already drops it on a database that ran every
-- migration in order, this makes sure of it.
DROP INDEX IF EXISTS jobs_run_proof_active_idx;

-- +goose Down
SELECT 1;
