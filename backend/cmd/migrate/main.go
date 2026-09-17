// Command migrate applies pending schema migrations. It connects with the
// migration role (which owns the schema) and never with arena_app.
package main

import (
	"log"
	"os"
	"time"

	"tolerance/internal/platform/db"
)

func main() {
	dsn := os.Getenv("ARENA_MIGRATE_DATABASE_URL")
	if dsn == "" {
		log.Fatal("ARENA_MIGRATE_DATABASE_URL must be set to a role that owns the schema")
	}
	if os.Getenv("ARENA_APP_ROLE_PASSWORD") == "" {
		log.Fatal("ARENA_APP_ROLE_PASSWORD must be set; migration 00001 uses it to (re)create the arena_app role")
	}
	start := time.Now()
	if err := db.Migrate(dsn); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Printf("migrations applied in %s", time.Since(start))
}
