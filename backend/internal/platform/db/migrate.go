package db

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver goose needs
	"github.com/pressly/goose/v3"

	"tolerance/migrations"
)

// Migrate applies every pending migration using the supplied DSN, which
// must be a role allowed to create roles, tables and policies (the
// migration role owns the schema; the application never does). It is safe
// to call repeatedly: goose tracks applied versions in its own table.
func Migrate(dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer sqlDB.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
