-- +goose Up

-- A user needs to see which organizations they belong to before any single
-- organization is "current" (the org switcher, GET /me). Postgres RLS OR's
-- multiple permissive policies together, so this adds a second, narrower
-- way into the same table alongside tenant_isolation: visible if it's your
-- own row, regardless of which organization (or no organization) is set for
-- this session. It grants no visibility into other members of an
-- organization the caller doesn't belong to, and — because WITH CHECK is
-- also OR'd — it would also allow inserting a membership row for yourself
-- into any organization by user_id alone; the invitation-acceptance command
-- guards that at the application level; no other command inserts into this
-- table without a superuser-owned bootstrap transaction.
CREATE POLICY self_visibility ON memberships
    USING (user_id = current_setting('app.user_id', true))
    WITH CHECK (user_id = current_setting('app.user_id', true));

-- +goose Down
DROP POLICY self_visibility ON memberships;
