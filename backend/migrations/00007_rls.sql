-- +goose Up

-- forge_app is the only role the running service connects as: no
-- BYPASSRLS, not the table owner (the migration role owns everything).
-- Row Level Security is a second barrier behind the role check every
-- handler already does in Go; a bug in that Go check must not be enough
-- to read or write another organization's row.

GRANT USAGE ON SCHEMA public TO forge_app;

GRANT SELECT, INSERT, UPDATE ON organizations, users TO forge_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON
    memberships, invitations,
    campaigns,
    missions, mission_versions, requirements,
    budgets,
    audit_events, idempotency_records, jobs
    TO forge_app;

-- organizations and users are not organization_id-scoped: an organization
-- is the tenant itself, and a user is a single identity that can belong to
-- several organizations through memberships, which IS scoped. RLS below
-- applies to every table that carries organization_id.
ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON memberships
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));

ALTER TABLE invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE invitations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invitations
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));

ALTER TABLE campaigns ENABLE ROW LEVEL SECURITY;
ALTER TABLE campaigns FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON campaigns
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));

ALTER TABLE missions ENABLE ROW LEVEL SECURITY;
ALTER TABLE missions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON missions
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));

ALTER TABLE mission_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE mission_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON mission_versions
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));

ALTER TABLE requirements ENABLE ROW LEVEL SECURITY;
ALTER TABLE requirements FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON requirements
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));

ALTER TABLE budgets ENABLE ROW LEVEL SECURITY;
ALTER TABLE budgets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON budgets
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));

ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_events
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));

ALTER TABLE idempotency_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE idempotency_records FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON idempotency_records
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));

ALTER TABLE jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON jobs
    USING (organization_id = current_setting('app.organization_id', true))
    WITH CHECK (organization_id = current_setting('app.organization_id', true));

-- +goose Down
DROP POLICY tenant_isolation ON jobs;
ALTER TABLE jobs DISABLE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON idempotency_records;
ALTER TABLE idempotency_records DISABLE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON audit_events;
ALTER TABLE audit_events DISABLE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON budgets;
ALTER TABLE budgets DISABLE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON requirements;
ALTER TABLE requirements DISABLE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON mission_versions;
ALTER TABLE mission_versions DISABLE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON missions;
ALTER TABLE missions DISABLE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON campaigns;
ALTER TABLE campaigns DISABLE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON invitations;
ALTER TABLE invitations DISABLE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON memberships;
ALTER TABLE memberships DISABLE ROW LEVEL SECURITY;

REVOKE ALL ON
    memberships, invitations,
    campaigns,
    missions, mission_versions, requirements,
    budgets,
    audit_events, idempotency_records, jobs
    FROM forge_app;
REVOKE ALL ON organizations, users FROM forge_app;
REVOKE USAGE ON SCHEMA public FROM forge_app;
