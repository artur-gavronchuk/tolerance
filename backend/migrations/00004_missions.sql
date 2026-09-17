-- +goose Up
CREATE TABLE missions (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations (id),
    campaign_id text NOT NULL REFERENCES campaigns (id),
    stage text NOT NULL CHECK (stage IN ('qualification', 'build', 'adapt', 'handoff')),
    ordinal int NOT NULL,
    state text NOT NULL DEFAULT 'draft' CHECK (state IN (
        'draft', 'calibrating', 'open', 'submission_closed', 'evaluating',
        'provisional', 'appeals_open', 'finalized', 'cancelled', 'invalidated'
    )),
    active_version_id text,
    version int NOT NULL DEFAULT 1,
    UNIQUE (campaign_id, ordinal)
);

CREATE TABLE mission_versions (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations (id),
    mission_id text NOT NULL REFERENCES missions (id),
    number int NOT NULL,
    parent_version_id text REFERENCES mission_versions (id),
    contract jsonb NOT NULL,
    contract_digest text,
    -- Interim slice-1 stand-in for scenario-based calibration (slice 2):
    -- an organizer's explicit confirmation gates calibrating → open until
    -- a real scenario bundle can be run against a reference submission.
    calibration_confirmed_at timestamptz,
    calibration_confirmed_by text REFERENCES users (id),
    published_at timestamptz,
    submission_deadline timestamptz,
    appeal_deadline timestamptz,
    policies jsonb NOT NULL DEFAULT '{}'::jsonb,
    version int NOT NULL DEFAULT 1,
    UNIQUE (mission_id, number)
);

ALTER TABLE missions
    ADD CONSTRAINT missions_active_version_fk
    FOREIGN KEY (active_version_id) REFERENCES mission_versions (id);

CREATE TABLE requirements (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations (id),
    mission_version_id text NOT NULL REFERENCES mission_versions (id),
    stable_key text NOT NULL,
    revision int NOT NULL DEFAULT 1,
    gate boolean NOT NULL,
    weight int NOT NULL DEFAULT 0,
    category text NOT NULL,
    text text NOT NULL,
    UNIQUE (mission_version_id, stable_key)
);

-- Once a version is published, its own row and its requirements are frozen.
-- The trigger rejects any UPDATE, not just specific columns: a published
-- contract is a fact about the past, not a record to be patched.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION forbid_update_to_published_mission_version()
RETURNS trigger AS $$
BEGIN
    IF OLD.published_at IS NOT NULL THEN
        RAISE EXCEPTION 'mission_versions % is published and immutable', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER mission_versions_immutable_once_published
    BEFORE UPDATE ON mission_versions
    FOR EACH ROW EXECUTE FUNCTION forbid_update_to_published_mission_version();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION forbid_update_to_published_requirement()
RETURNS trigger AS $$
DECLARE
    is_published boolean;
BEGIN
    SELECT (published_at IS NOT NULL) INTO is_published
    FROM mission_versions WHERE id = OLD.mission_version_id;
    IF is_published THEN
        RAISE EXCEPTION 'requirement % belongs to a published mission_version and is immutable', OLD.id
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER requirements_immutable_once_published
    BEFORE UPDATE ON requirements
    FOR EACH ROW EXECUTE FUNCTION forbid_update_to_published_requirement();

-- +goose Down
DROP TRIGGER requirements_immutable_once_published ON requirements;
DROP FUNCTION forbid_update_to_published_requirement();
DROP TRIGGER mission_versions_immutable_once_published ON mission_versions;
DROP FUNCTION forbid_update_to_published_mission_version();
DROP TABLE requirements;
ALTER TABLE missions DROP CONSTRAINT missions_active_version_fk;
DROP TABLE mission_versions;
DROP TABLE missions;
