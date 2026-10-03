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
	"tolerance/internal/tasks"
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

	if err := syncTasks(context.Background(), pool); err != nil {
		log.Fatalf("tasks: %v", err)
	}

	if err := games.Sync(context.Background(), pool); err != nil {
		log.Fatalf("games: %v", err)
	}
	log.Printf("games: house bots synced")
}

// syncTasks loads the task catalog and upserts it. ARENA_SKILLS_DIR is <language>/<task>/ with a skill.json per
// language (the private rating catalog mounts here).
func syncTasks(ctx context.Context, pool *db.Pool) error {
	var lists [][]tasks.Task
	if dir := os.Getenv("ARENA_SKILLS_DIR"); dir != "" {
		ts, err := tasks.LoadByLanguage(dir)
		if err != nil {
			return err
		}
		lists = append(lists, ts)
	}
	all, err := tasks.Merge(lists...)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		// An empty or unmounted directory must not deactivate the whole pool.
		log.Printf("tasks: no catalog configured or empty, tasks left as they are")
		return nil
	}
	if err := tasks.Sync(ctx, pool, all); err != nil {
		return err
	}
	log.Printf("tasks: %d task(s) synced", len(all))
	return nil
}
