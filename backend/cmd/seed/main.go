// Command seed fills a LOCAL database with deterministic fake activity over the last 30 days, so pages and
// queries can be looked at (and measured) with realistic volume: ~500 people, daily submissions, product
// weeks with entries and votes, tanks bots with ratings, matches and tournaments, notifications.
//
//	go run ./cmd/seed --yes            (make seed)
//
// It writes straight to the database with the migration role and refuses anything but localhost. Everything it
// creates hangs off users @seed.local (and ids prefixed seed_), so a re-run first removes its own rows.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
)

const (
	emailDomain = "@seed.local"
	idPrefix    = "seed_"
)

func main() {
	yes := flag.Bool("yes", false, "confirm: wipe previous seeded rows and insert new ones")
	nUsers := flag.Int("users", 500, "number of fake people")
	flag.Parse()

	dsn := os.Getenv("ARENA_MIGRATE_DATABASE_URL")
	if dsn == "" {
		log.Fatal("ARENA_MIGRATE_DATABASE_URL must be set (see make seed)")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		log.Fatalf("bad ARENA_MIGRATE_DATABASE_URL: %v", err)
	}
	if h := u.Hostname(); h != "localhost" && h != "127.0.0.1" {
		log.Fatalf("refusing to seed %q: only localhost / 127.0.0.1 databases are allowed", h)
	}
	if !*yes {
		log.Fatalf("this deletes earlier seeded rows (users %s*) and inserts fake data into %s; pass --yes to go on", emailDomain, u.Host+u.Path)
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	start := time.Now()
	s := newSeeder(*nUsers)
	err = pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		steps := []struct {
			name string
			fn   func(context.Context, pgx.Tx) error
		}{
			{"cleanup", s.cleanup},
			{"users", s.users},
			{"daily", s.daily},
			{"products", s.products},
			{"tanks", s.tanks},
			{"notifications", s.notifications},
		}
		for _, st := range steps {
			t := time.Now()
			if err := st.fn(ctx, tx); err != nil {
				return fmt.Errorf("%s: %w", st.name, err)
			}
			log.Printf("%-14s %s", st.name, time.Since(t).Round(time.Millisecond))
		}
		_, err := tx.Exec(ctx, `ANALYZE`)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("seeded in %s: %s", time.Since(start).Round(time.Millisecond), s.summary())
	log.Printf("sign in locally as %s (dev login) to see a populated profile", s.usersList[0].email)
}

func (s *seeder) summary() string {
	var b strings.Builder
	for _, k := range []string{"users", "submissions", "product_entries", "product_votes", "product_judgments", "bots", "matches", "tournaments", "notifications"} {
		fmt.Fprintf(&b, "%s=%d ", k, s.counts[k])
	}
	return strings.TrimSpace(b.String())
}
