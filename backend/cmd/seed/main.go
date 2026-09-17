// Command seed fills an empty database with the frontend prototype data.
package main

import (
	"context"
	"log"
	"os"

	"tolerance/fixtures/seed"
	"tolerance/internal/platform/db"
)

func main() {
	dsn := os.Getenv("ARENA_MIGRATE_DATABASE_URL")
	if dsn == "" {
		log.Fatal("ARENA_MIGRATE_DATABASE_URL must be set")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := seed.Load(ctx, pool); err != nil {
		log.Fatal(err)
	}
	log.Print("seed loaded")
}
