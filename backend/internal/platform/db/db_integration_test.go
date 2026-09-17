package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/dbtest"
)

func TestMigrate_AppliesCleanlyAndTwiceWithoutError(t *testing.T) {
	d := dbtest.New(t) // migrates once
	// Deploys re-run migrations unconditionally; a second call against an
	// already-migrated database must be a no-op, not an error.
	if err := db.Migrate(d.AdminDSN); err != nil {
		t.Fatalf("expected re-running migrations to be idempotent, got: %v", err)
	}
	ctx := context.Background()
	err := d.AdminPool.GlobalTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var n int
		return tx.QueryRow(ctx, "SELECT count(*) FROM organizations").Scan(&n)
	})
	if err != nil {
		t.Fatalf("expected organizations table to exist after migration: %v", err)
	}
}

func TestForgeAppRole_HasNoBypassAndNoSuperuser(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	var rolsuper, rolbypassrls bool
	err := d.AdminPool.GlobalTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'forge_app'`).Scan(&rolsuper, &rolbypassrls)
	})
	if err != nil {
		t.Fatalf("query forge_app role: %v", err)
	}
	if rolsuper || rolbypassrls {
		t.Fatalf("forge_app must have neither SUPERUSER nor BYPASSRLS, got super=%v bypassrls=%v", rolsuper, rolbypassrls)
	}
}

func TestRLS_CrossOrganizationRowIsInvisibleEvenThoughItExists(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()

	// Arrange two organizations and a campaign belonging to org A, using
	// the admin (owner) connection which is not subject to RLS.
	err := d.AdminPool.GlobalTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ('org_a', 'A'), ('org_b', 'B')`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO campaigns (id, organization_id, name, mode, currency)
			VALUES ('camp_a', 'org_a', 'Season A', 'private_trial', 'USD')`)
		return err
	})
	if err != nil {
		t.Fatalf("arrange fixtures: %v", err)
	}

	// Act: connect as forge_app (the role the running service uses) scoped
	// to org B, and try to read org A's campaign by its exact, known id.
	var rowCount int
	err = d.AppPool.Tx(ctx, "org_b", "", func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM campaigns WHERE id = 'camp_a'`).Scan(&rowCount)
	})
	if err != nil {
		t.Fatalf("query as forge_app: %v", err)
	}
	if rowCount != 0 {
		t.Fatalf("expected org B's session to see zero rows of org A's campaign, got %d", rowCount)
	}

	// Control: the same query scoped to the owning organization does see it.
	err = d.AppPool.Tx(ctx, "org_a", "", func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM campaigns WHERE id = 'camp_a'`).Scan(&rowCount)
	})
	if err != nil {
		t.Fatalf("query as forge_app for the owning org: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("expected org A's own session to see its campaign, got %d", rowCount)
	}
}

func TestRLS_ForgeAppCannotInsertARowIntoAnotherOrganization(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	err := d.AdminPool.GlobalTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ('org_a', 'A'), ('org_b', 'B')`)
		return err
	})
	if err != nil {
		t.Fatalf("arrange organizations: %v", err)
	}

	err = d.AppPool.Tx(ctx, "org_a", "", func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO campaigns (id, organization_id, name, mode, currency)
			VALUES ('camp_x', 'org_b', 'Smuggled', 'private_trial', 'USD')`)
		return err
	})
	if err == nil {
		t.Fatalf("expected the WITH CHECK policy to reject inserting a row tagged with a different organization")
	}
}

func TestMissionVersionsImmutableOncePublished(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	err := d.AdminPool.GlobalTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		stmts := []string{
			`INSERT INTO organizations (id, name) VALUES ('org_a', 'A')`,
			`INSERT INTO campaigns (id, organization_id, name, mode, currency) VALUES ('camp_a', 'org_a', 'Season', 'private_trial', 'USD')`,
			`INSERT INTO missions (id, organization_id, campaign_id, stage, ordinal, state) VALUES ('mission_a', 'org_a', 'camp_a', 'build', 1, 'open')`,
			`INSERT INTO mission_versions (id, organization_id, mission_id, number, contract, contract_digest, published_at)
				VALUES ('mv_a', 'org_a', 'mission_a', 1, '{}'::jsonb, 'sha256:x', now())`,
		}
		for _, s := range stmts {
			if _, err := tx.Exec(ctx, s); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("arrange a published mission version: %v", err)
	}

	err = d.AdminPool.GlobalTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE mission_versions SET contract = '{"tampered":true}'::jsonb WHERE id = 'mv_a'`)
		return err
	})
	if err == nil {
		t.Fatalf("expected updating a published mission_version to be rejected by the immutability trigger")
	}
}

func TestRequirementsImmutableOncePublished(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	err := d.AdminPool.GlobalTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		stmts := []string{
			`INSERT INTO organizations (id, name) VALUES ('org_a', 'A')`,
			`INSERT INTO campaigns (id, organization_id, name, mode, currency) VALUES ('camp_a', 'org_a', 'Season', 'private_trial', 'USD')`,
			`INSERT INTO missions (id, organization_id, campaign_id, stage, ordinal, state) VALUES ('mission_a', 'org_a', 'camp_a', 'build', 1, 'open')`,
			`INSERT INTO mission_versions (id, organization_id, mission_id, number, contract, contract_digest, published_at)
				VALUES ('mv_a', 'org_a', 'mission_a', 1, '{}'::jsonb, 'sha256:x', now())`,
			`INSERT INTO requirements (id, organization_id, mission_version_id, stable_key, gate, category, text)
				VALUES ('req_a', 'org_a', 'mv_a', 'B-G2', true, 'gate', 'Маршрут выполним.')`,
		}
		for _, s := range stmts {
			if _, err := tx.Exec(ctx, s); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("arrange a requirement on a published mission version: %v", err)
	}

	err = d.AdminPool.GlobalTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE requirements SET text = 'tampered' WHERE id = 'req_a'`)
		return err
	})
	if err == nil {
		t.Fatalf("expected updating a requirement of a published mission_version to be rejected by the immutability trigger")
	}
}
