-- +goose Up
CREATE TABLE audit_events (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations (id),
    actor_id text NOT NULL,
    actor_kind text NOT NULL DEFAULT 'user' CHECK (actor_kind IN ('user', 'service')),
    action text NOT NULL,
    aggregate_kind text NOT NULL,
    aggregate_id text NOT NULL,
    before_version int,
    after_version int,
    reason text,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    request_id text,
    at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_org_at_idx ON audit_events (organization_id, at DESC);

CREATE TABLE idempotency_records (
    organization_id text NOT NULL REFERENCES organizations (id),
    actor_id text NOT NULL,
    endpoint text NOT NULL,
    key text NOT NULL,
    payload_digest text NOT NULL,
    status int NOT NULL,
    body jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (organization_id, actor_id, endpoint, key)
);

-- Empty in slice 1; the dispatcher, runner and evaluator claim rows from
-- this table starting in slices 2 and 4. Created now so its shape is part
-- of the same reviewed migration history as everything that references it.
CREATE TABLE jobs (
    id text PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('run_attempt', 'evaluation', 'publication', 'retention', 'reconcile')),
    aggregate_kind text NOT NULL,
    aggregate_id text NOT NULL,
    organization_id text NOT NULL REFERENCES organizations (id),
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'leased', 'done', 'failed', 'dead')),
    run_after timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_until timestamptz,
    fencing_token bigint NOT NULL DEFAULT 0,
    attempts int NOT NULL DEFAULT 0,
    max_attempts int NOT NULL DEFAULT 1,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX jobs_claim_idx ON jobs (state, run_after);

-- +goose Down
DROP TABLE jobs;
DROP TABLE idempotency_records;
DROP TABLE audit_events;
