package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/dbtest"
)

func TestMigrate_CreatesSliceTablesAndAppRoleCanUseThem(t *testing.T) {
	d := dbtest.New(t)
	for _, table := range []string{"users", "sessions", "agents", "api_keys", "agent_presence", "proof_tasks", "proofs", "jobs", "audit_events"} {
		err := d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "SELECT 1 FROM "+table+" LIMIT 1")
			return err
		})
		if err != nil {
			t.Fatalf("arena_app cannot read %s: %v", table, err)
		}
	}
}

// Several replicas (api and worker containers, or several of either) can
// start at once and each try to migrate on boot. Migrate takes a Postgres
// session-level advisory lock around goose's Up so concurrent callers
// serialize instead of racing the same DDL. A second call against an
// already-migrated database (nothing pending) must still succeed cleanly
// through the lock/unlock path, which is what a second replica sees in
// practice once the first has finished.
func TestMigrate_SecondCallAfterMigrationSucceeds(t *testing.T) {
	d := dbtest.New(t) // already fully migrated once by New itself
	if err := db.Migrate(d.AdminDSN); err != nil {
		t.Fatalf("second migrate call: %v", err)
	}
}
