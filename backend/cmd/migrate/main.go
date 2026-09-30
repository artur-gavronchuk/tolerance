// Command migrate applies pending schema migrations. It connects with the
// migration role (which owns the schema) and never with arena_app.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"tolerance/internal/games"
	"tolerance/internal/platform/db"
	"tolerance/internal/proofs"
	"tolerance/internal/skills"
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

	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer pool.Close()

	if dir := os.Getenv("ARENA_PROOFS_DIR"); dir != "" {
		tasks, err := proofs.LoadCatalog(dir)
		if err != nil {
			log.Fatalf("catalog: %v", err)
		}
		if err := proofs.SyncCatalog(context.Background(), pool, tasks); err != nil {
			log.Fatalf("catalog: %v", err)
		}
		log.Printf("catalog: %d task(s) synced", len(tasks))
	}

	if dir := os.Getenv("ARENA_SKILLS_DIR"); dir != "" {
		sk, tasks, err := skills.LoadCatalog(dir)
		if err != nil {
			log.Fatalf("skills: %v", err)
		}
		if len(sk) == 0 {
			// An empty or unmounted directory must not deactivate the whole task pool.
			log.Printf("skills: %s has no skills, catalog left as it is", dir)
		} else {
			if err := skills.SyncCatalog(context.Background(), pool, sk, tasks); err != nil {
				log.Fatalf("skills: %v", err)
			}
			log.Printf("skills: %d skill(s), %d task(s) synced", len(sk), len(tasks))
		}
	}

	if err := games.Sync(context.Background(), pool); err != nil {
		log.Fatalf("games: %v", err)
	}
	log.Printf("games: house bots and tanks-bot task synced")
}
