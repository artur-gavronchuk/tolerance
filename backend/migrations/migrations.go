// Package migrations embeds the schema migrations and registers the
// migrations that need application logic (creating the least-privileged
// arena_app and arena_worker roles with passwords taken from the
// environment, never written into a SQL file). cmd/api and tests both
// import this package for its embedded filesystem and side-effecting
// registration.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
