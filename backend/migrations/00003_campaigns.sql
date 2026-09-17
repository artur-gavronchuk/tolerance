-- +goose Up
CREATE TABLE campaigns (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations (id),
    name text NOT NULL,
    slug text,
    mode text NOT NULL CHECK (mode IN ('public_season', 'private_trial')),
    publication_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
    currency text NOT NULL,
    state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'active', 'finalizing', 'completed', 'cancelled')),
    version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- A slug is only meaningful (and only needs to be unique) for a public
-- season; a private trial never gets a public URL.
CREATE UNIQUE INDEX campaigns_slug_key ON campaigns (slug) WHERE slug IS NOT NULL;

-- +goose Down
DROP TABLE campaigns;
