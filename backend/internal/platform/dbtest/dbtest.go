// Package dbtest starts a disposable PostgreSQL container, applies the real
// migrations against it, and hands integration tests both an admin
// connection (for arranging fixtures) and the same arena_app connection the
// running service uses.
package dbtest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"tolerance/internal/platform/db"
)

const appTestPassword = "arena_app_test_password" // ephemeral per-container, not a real secret

// DB is a fully migrated, disposable database plus both roles' pools.
type DB struct {
	AdminPool *db.Pool // migration/owner role, for arranging fixtures and asserting things
	AppPool   *db.Pool // arena_app; exactly what cmd/api connects as
	AdminDSN  string
	AppDSN    string
}

// New starts a container, migrates it, and returns ready-to-use pools. It
// skips the test (rather than failing it) if Docker is not reachable, since
// that is an environment limitation, not a code defect.
func New(t *testing.T) *DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("arena"),
		tcpostgres.WithUsername("arena_migrate"),
		tcpostgres.WithPassword("arena_migrate_test_password"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Skipf("postgres testcontainer unavailable, skipping integration test: %v", err)
	}
	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
	})

	adminDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	t.Setenv("ARENA_APP_ROLE_PASSWORD", appTestPassword)
	if err := db.Migrate(adminDSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("mapped port: %v", err)
	}
	appDSN := fmt.Sprintf("postgres://arena_app:%s@%s:%s/arena?sslmode=disable", appTestPassword, host, port.Port())

	adminPool, err := db.Open(ctx, adminDSN)
	if err != nil {
		t.Fatalf("open admin pool: %v", err)
	}
	t.Cleanup(adminPool.Close)

	appPool, err := db.Open(ctx, appDSN)
	if err != nil {
		t.Fatalf("open app pool: %v", err)
	}
	t.Cleanup(appPool.Close)

	return &DB{AdminPool: adminPool, AppPool: appPool, AdminDSN: adminDSN, AppDSN: appDSN}
}
