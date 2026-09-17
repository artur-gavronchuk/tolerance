-- +goose Up
CREATE TABLE budgets (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations (id),
    scope_kind text NOT NULL CHECK (scope_kind IN ('campaign', 'entry', 'organizer_reserve')),
    scope_id text NOT NULL,
    currency text NOT NULL,
    limit_minor bigint NOT NULL CHECK (limit_minor >= 0),
    version int NOT NULL DEFAULT 1,
    UNIQUE (organization_id, scope_kind, scope_id)
);

-- Reservations and usage entries are introduced in slice 2 alongside
-- submissions and runs, which are what actually consume a budget; slice 1
-- only needs a limit to exist and be readable.

-- +goose Down
DROP TABLE budgets;
