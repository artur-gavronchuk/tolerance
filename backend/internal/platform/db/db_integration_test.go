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
	for _, table := range []string{"users", "sessions", "agents", "api_keys", "agent_presence", "proof_tasks", "proofs", "jobs", "audit_events", "agent_versions", "skills", "skill_tasks", "qualification_runs", "skill_ratings"} {
		err := d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "SELECT 1 FROM "+table+" LIMIT 1")
			return err
		})
		if err != nil {
			t.Fatalf("arena_app cannot read %s: %v", table, err)
		}
	}
}

// TestMigrate_WorkerRoleCanReadItsOwnTables is arena_app's own test above, for arena_worker: every table
// the worker role's code paths touch (internal/proofs.Worker, internal/games.Worker) must be readable
// through 00007_worker_grants.sql's grants. agents is intentionally not in this list - arena_worker only
// has column-level SELECT on it (id, owner_user_id, name); SELECT * would fail, which is the point.
func TestMigrate_WorkerRoleCanReadItsOwnTables(t *testing.T) {
	d := dbtest.New(t)
	for _, table := range []string{
		"jobs", "proofs", "proof_tasks",
		"game_bots", "bot_versions", "matches", "match_players", "match_replays", "tanks_broadcasts",
	} {
		err := d.WorkerPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "SELECT 1 FROM "+table+" LIMIT 1")
			return err
		})
		if err != nil {
			t.Fatalf("arena_worker cannot read %s: %v", table, err)
		}
	}
	// The three columns the worker's own code actually reads off agents (see games.agentNameFor and
	// proofs.Worker's GameBotJudge path).
	err := d.WorkerPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "SELECT id, owner_user_id, name FROM agents LIMIT 1")
		return err
	})
	if err != nil {
		t.Fatalf("arena_worker cannot read agents(id, owner_user_id, name): %v", err)
	}
	// audit_events is INSERT-only for the worker (it never reads the audit log back).
	err = d.WorkerPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO audit_events (id, actor_id, actor_kind, action, aggregate_kind, aggregate_id, payload)
			VALUES ('audit_test1', 'system', 'system', 'test.probe', 'test', 'x', '{}'::jsonb)`)
		return err
	})
	if err != nil {
		t.Fatalf("arena_worker cannot insert into audit_events: %v", err)
	}
}

// TestMigrate_WorkerRoleCannotReachUserSecrets is the negative counterpart: a sandbox escape on a worker
// host must not hand out session cookies, OAuth identities or API key hashes, so arena_worker must not be
// able to read (or write) any of the tables that hold them, or the full agents row (description/version
// aren't worker-readable either, only the three columns above).
func TestMigrate_WorkerRoleCannotReachUserSecrets(t *testing.T) {
	d := dbtest.New(t)
	for _, table := range []string{"users", "sessions", "user_identities", "api_keys", "agent_presence"} {
		err := d.WorkerPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "SELECT 1 FROM "+table+" LIMIT 1")
			return err
		})
		if err == nil {
			t.Fatalf("arena_worker must not be able to read %s", table)
		}
	}
	err := d.WorkerPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "SELECT description FROM agents LIMIT 1")
		return err
	})
	if err == nil {
		t.Fatal("arena_worker must not be able to read agents.description")
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
