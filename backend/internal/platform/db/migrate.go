package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver goose needs
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"tolerance/migrations"
)

// Migrate applies every pending migration using the supplied DSN, which
// must be a role allowed to create roles, tables and policies (the
// migration role owns the schema; the application never does). It is safe
// to call repeatedly: goose tracks applied versions in its own table.
//
// Several replicas can start at once (a horizontally scaled deploy brings
// up api and worker together), so this must be safe under concurrent
// callers. It takes a Postgres session-level advisory lock, via goose's own
// lock.SessionLocker, on a dedicated connection before running the
// migrations: a second concurrent Migrate blocks (goose retries the lock
// for a few minutes) until the first one finishes and releases it, instead
// of both racing the same DDL. The lock only needs to be held around the
// call, not the specific connection the DDL runs on: an advisory lock's
// mutual exclusion is visible cluster-wide regardless of which connection
// does the protected work, as long as it is acquired and released on one
// consistent session, which is exactly what SessionLock/SessionUnlock do.
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

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create migration lock: %w", err)
	}
	ctx := context.Background()
	lockConn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration lock connection: %w", err)
	}
	defer lockConn.Close()
	if err := locker.SessionLock(ctx, lockConn); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() { _ = locker.SessionUnlock(context.WithoutCancel(ctx), lockConn) }()

	if err := goose.Up(sqlDB, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
