-- +goose Up
CREATE TABLE organizations (
    id text PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id text PRIMARY KEY,
    oidc_issuer text NOT NULL,
    oidc_subject text NOT NULL,
    email text,
    display_name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (oidc_issuer, oidc_subject)
);

CREATE TABLE memberships (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations (id),
    user_id text NOT NULL REFERENCES users (id),
    role text NOT NULL CHECK (role IN ('owner', 'organizer', 'participant', 'evaluator', 'arbiter', 'viewer')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    invited_by text REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, user_id)
);

CREATE TABLE invitations (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations (id),
    email text NOT NULL,
    role text NOT NULL CHECK (role IN ('owner', 'organizer', 'participant', 'evaluator', 'arbiter', 'viewer')),
    token_digest text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE invitations;
DROP TABLE memberships;
DROP TABLE users;
DROP TABLE organizations;
