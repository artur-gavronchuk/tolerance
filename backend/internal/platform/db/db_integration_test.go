package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

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
