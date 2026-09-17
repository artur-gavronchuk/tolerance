// Package migrations embeds the schema migrations and registers the one
// migration that needs application logic (creating the least-privileged
// application role with a password taken from the environment, never
// written into a SQL file). cmd/api and tests both import this package
// for its embedded filesystem and side-effecting registration.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
