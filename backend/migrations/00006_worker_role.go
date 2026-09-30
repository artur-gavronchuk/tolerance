package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upWorkerRole, downWorkerRole)
}

// The sandbox worker (ARENA_ROLE=worker) connects as arena_worker instead of arena_app: it executes
// untrusted agent diffs and tanks bots in Docker, and a container escape on a worker host must not hand
// out arena_app's credentials (full application access, including sessions, OAuth identities and API key
// hashes - see CLAUDE.md). Its password, like arena_app's, is never written into a migration file.
//
// Unlike arena_app, ARENA_WORKER_ROLE_PASSWORD is allowed to be empty: the production .env this ships
// against does not have it yet (it is added by hand before this migration's grants matter there), and
// dev/CI compose files don't set it either - ARENA_WORKER_DATABASE_URL then falls back to
// ARENA_APP_DATABASE_URL (see cmd/api's config), so nothing actually connects as arena_worker until an
// operator sets the password. An empty password leaves the role NOLOGIN rather than failing the whole
// migration, which would otherwise block every deploy that hasn't rotated this in yet.
const workerRolePasswordEnv = "ARENA_WORKER_ROLE_PASSWORD"

func upWorkerRole(ctx context.Context, tx *sql.Tx) error {
	password := os.Getenv(workerRolePasswordEnv)
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'arena_worker')`).Scan(&exists); err != nil {
		return err
	}
	if password == "" {
		log.Printf("migration 00006: %s is not set; arena_worker will be left NOLOGIN (no password) until it is - "+
			"the worker role falls back to arena_app in the meantime (see cmd/api's config)", workerRolePasswordEnv)
		if exists {
			_, err := tx.ExecContext(ctx, `ALTER ROLE arena_worker WITH NOLOGIN PASSWORD NULL NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`)
			return err
		}
		_, err := tx.ExecContext(ctx, `CREATE ROLE arena_worker WITH NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`)
		return err
	}
	if exists {
		// Keep the password in sync with the environment on every deploy, same as arena_app.
		_, err := tx.ExecContext(ctx, fmt.Sprintf(`ALTER ROLE arena_worker WITH LOGIN PASSWORD %s NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`, quoteLiteral(password)))
		return err
	}
	_, err := tx.ExecContext(ctx, fmt.Sprintf(`CREATE ROLE arena_worker WITH LOGIN PASSWORD %s NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`, quoteLiteral(password)))
	return err
}

func downWorkerRole(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP ROLE IF EXISTS arena_worker`)
	return err
}
