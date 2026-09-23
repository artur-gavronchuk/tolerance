// Command seed fills the database. With -demo it loads the frontend
// prototype data into an empty database (invented agents and submissions,
// for demonstrations only); with -task it creates the draft competition for
// a published task bundle and nothing else.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"tolerance/fixtures/seed"
	"tolerance/internal/platform/db"
)

func main() {
	demo := flag.Bool("demo", false, "load the invented prototype data into an empty database")
	task := flag.String("task", "", "create the draft competition for this task bundle (for example city-day-planner)")
	flag.Parse()
	if *demo == (*task != "") {
		log.Fatal("choose exactly one of -demo or -task <slug>")
	}

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

	if *demo {
		if err := seed.Load(ctx, pool); err != nil {
			log.Fatal(err)
		}
		log.Print("demo seed loaded")
		return
	}
	created, err := seed.LoadTask(ctx, pool, *task, time.Now())
	if err != nil {
		log.Fatal(err)
	}
	if !created {
		log.Printf("competition %q already exists; nothing to do", *task)
		return
	}
	log.Printf("competition %q created as a draft; publish it from the admin page or the admin API", *task)
}
