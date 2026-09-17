package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/dbtest"
)

func TestMigrate_AppliesCleanlyAndTwice(t *testing.T) {
	d := dbtest.New(t)
	if err := db.Migrate(d.AdminDSN); err != nil {
		t.Fatalf("second migrate must be a no-op, got: %v", err)
	}
	ctx := context.Background()
	for _, table := range []string{"users", "agents", "api_keys", "competitions", "submissions", "judgments",
		"matches", "match_events", "arena_queue", "agent_badges", "jobs", "audit_events", "idempotency_records"} {
		err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var n int
			return tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
		})
		if err != nil {
			t.Fatalf("arena_app must be able to read %s: %v", table, err)
		}
	}
	for _, view := range []string{"competition_rankings", "agent_standings"} {
		err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var n int
			return tx.QueryRow(ctx, "SELECT count(*) FROM "+view).Scan(&n)
		})
		if err != nil {
			t.Fatalf("arena_app must be able to read view %s: %v", view, err)
		}
	}
}

func TestAppRole_HasNoBypassAndNoSuperuser(t *testing.T) {
	d := dbtest.New(t)
	var rolsuper, rolbypassrls bool
	err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'arena_app'`).Scan(&rolsuper, &rolbypassrls)
	})
	if err != nil {
		t.Fatalf("query role: %v", err)
	}
	if rolsuper || rolbypassrls {
		t.Fatalf("arena_app must be neither superuser nor bypassrls, got super=%v bypass=%v", rolsuper, rolbypassrls)
	}
}

func TestCompetitions_ImmutableAfterPublish(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name) VALUES ('user_a', 'seed', 'a', 'a', 'A')`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO competitions (id, slug, title, summary, brief, category, difficulty, status, points, deadline, criteria, created_by, published_at)
			VALUES ('comp_a', 'a', 'A', 's', 'b', 'Bug fix', 'Easy', 'active', 100, now() + interval '1 day',
			        '[{"name":"Tests","weight":100,"description":"d"}]', 'user_a', now())`)
		return err
	})
	if err != nil {
		t.Fatalf("arrange: %v", err)
	}
	err = d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE competitions SET points = 200 WHERE id = 'comp_a'`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("expected trigger to reject the update with an 'immutable' error, got: %v", err)
	}
	// Operational columns stay writable.
	err = d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE competitions SET status = 'closed', closed_at = now(), version = version + 1 WHERE id = 'comp_a'`)
		return err
	})
	if err != nil {
		t.Fatalf("closing a published competition must be allowed: %v", err)
	}
}
