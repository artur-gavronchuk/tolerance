package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upForgeAppRole, downForgeAppRole)
}

// The application connects as forge_app: no BYPASSRLS, no table ownership.
// Its password is never written into a migration file; it must be supplied
// out of band (docker-compose env for development, a secrets manager in
// production) so that `git log` on this repository never contains a real
// credential.
const forgeAppRolePasswordEnv = "FORGE_APP_ROLE_PASSWORD"

func upForgeAppRole(ctx context.Context, tx *sql.Tx) error {
	password := os.Getenv(forgeAppRolePasswordEnv)
	if password == "" {
		return fmt.Errorf("migration 00001: %s must be set before running migrations", forgeAppRolePasswordEnv)
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'forge_app')`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		// Keep the password in sync with the environment on every deploy
		// rather than assuming it never rotates.
		_, err := tx.ExecContext(ctx, fmt.Sprintf(`ALTER ROLE forge_app WITH LOGIN PASSWORD %s NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`, quoteLiteral(password)))
		return err
	}
	_, err := tx.ExecContext(ctx, fmt.Sprintf(`CREATE ROLE forge_app WITH LOGIN PASSWORD %s NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`, quoteLiteral(password)))
	return err
}

func downForgeAppRole(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP ROLE IF EXISTS forge_app`)
	return err
}

// quoteLiteral escapes a string for use as a standard-conforming SQL string
// literal. Role passwords cannot be passed as bind parameters in DDL, so
// values are escaped by doubling embedded single quotes; Postgres defaults
// to standard_conforming_strings = on, so backslashes need no special
// handling.
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
