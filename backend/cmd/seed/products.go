package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type productWeek struct {
	slug   string
	offset int // weeks back from the current one
}

// Five weeks: this week's task is open (entries hidden), the four before it are final. Kinds alternate the
// way the rotation does.
var productWeeks = []productWeek{
	{"contrast-checker", 4}, {"calc", 3}, {"habit-tracker", 2}, {"cronnext", 1}, {"kanban-board", 0},
}

func tinyZip(name, body string) []byte {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create(name)
	_, _ = f.Write([]byte(body))
	_ = w.Close()
	return b.Bytes()
}

type seedEntry struct {
	id, user string
	quality  float64
	done     bool
	at       time.Time
}

func weekStartOf(t time.Time) time.Time {
	back := (int(t.Weekday()) + 6) % 7
	return time.Date(t.Year(), t.Month(), t.Day()-back, 0, 0, 0, 0, time.UTC)
}

func (s *seeder) products(ctx context.Context, tx pgx.Tx) error {
	siteZip := tinyZip("index.html", "<!doctype html><title>demo</title><h1>Seeded demo entry</h1>")
	cliZip := tinyZip("main.py", "print('seeded demo entry')\n")
	ws := weekStartOf(s.now)
	var entryRows, voteRows, judgeRows [][]any
	n := 0
	for _, w := range productWeeks {
		var kind string
		var names []string
		var hasBench bool
		err := tx.QueryRow(ctx, `SELECT kind, coalesce((SELECT array_agg(x ->> 'name') FROM jsonb_array_elements(scenarios) x), '{}'), bench IS NOT NULL
			FROM product_tasks WHERE slug = $1`, w.slug).Scan(&kind, &names, &hasBench)
		if err != nil {
			return fmt.Errorf("product task %s: %w", w.slug, err)
		}
		opens := ws.AddDate(0, 0, -7*w.offset)
		deadline := opens.AddDate(0, 0, 7)
		if _, err := tx.Exec(ctx, `UPDATE product_tasks SET opens_at = $2, deadline = $3, frozen = false WHERE slug = $1`, w.slug, opens, deadline); err != nil {
			return err
		}
		end := deadline
		if end.After(s.now) {
			end = s.now.Add(-time.Minute)
		}
		window := int(end.Sub(opens).Minutes())

		var entries []seedEntry
		best := map[string]seedEntry{} // each person's best done entry
		for ui := range s.usersList {
			u := &s.usersList[ui]
			if u.created.After(end) || s.rng.Float64() > 0.12+0.55*u.activity {
				continue
			}
			attempts := 1 + s.rng.Intn(3)
			at := opens.Add(time.Duration(s.rng.Intn(window)) * time.Minute)
			stack := u.stacks[0]
			for k := 0; k < attempts; k++ {
				at = at.Add(time.Duration(10+s.rng.Intn(600)) * time.Minute)
				if at.After(end) {
					break
				}
				q := math.Min(1, math.Max(0, u.skill+stackBonus(stack)+0.08*float64(k)+0.15*s.rng.NormFloat64()))
				id := fmt.Sprintf("%se%d", idPrefix, n)
				n++
				status, passed, total := "done", 0, len(names)
				var reason *string
				results := "[]"
				var bench, spread *float64
				if s.rng.Float64() < 0.02 {
					status, reason = "infra_error", ptr("sandbox_unavailable")
					total = 0
				} else if total > 0 {
					var rs []string
					for i, nm := range names {
						ok := s.rng.Float64() < 0.35+0.65*q
						if kind == "cli" && q > 0.9 {
							ok = true
						}
						if ok {
							passed++
						}
						rs = append(rs, fmt.Sprintf(`{"name":%q,"passed":%t}`, fmt.Sprintf("%s-%d", nm, i), ok))
					}
					results = "[" + strings.Join(rs, ",") + "]"
					if hasBench && passed == total {
						b := 25 + 120*(1-q)*s.rng.Float64() + 10*s.rng.Float64()
						sp := b * (0.02 + 0.06*s.rng.Float64())
						bench, spread = &b, &sp
					}
				}
				zipData := siteZip
				if kind == "cli" {
					zipData = cliZip
				}
				finished := at.Add(time.Duration(20+s.rng.Intn(100)) * time.Second)
				entryRows = append(entryRows, []any{id, w.slug, u.id, zipData, stack, status, passed, total, reason, results, "", at, finished, bench, spread})
				e := seedEntry{id: id, user: u.id, quality: q + float64(passed)*0.0001, done: status == "done", at: at}
				entries = append(entries, e)
				if e.done && e.quality > best[u.id].quality {
					best[u.id] = e
				}
			}
		}
		s.counts["product_entries"] += len(entries)
		if w.offset == 0 {
			continue // still open: nothing is public, so no votes yet
		}

		votingEnds := deadline.Add(3 * 24 * time.Hour)
		voteAt := func() time.Time {
			t := deadline.Add(time.Duration(s.rng.Intn(3*24*60)) * time.Minute)
			if t.After(s.now) {
				t = s.now.Add(-time.Minute)
			}
			return t
		}
		_ = votingEnds
		var ranked []seedEntry
		for _, e := range best {
			ranked = append(ranked, e)
		}
		sort.Slice(ranked, func(i, j int) bool {
			if ranked[i].quality != ranked[j].quality {
				return ranked[i].quality > ranked[j].quality
			}
			return ranked[i].id < ranked[j].id
		})
		if len(ranked) < 3 {
			continue
		}
		if kind == "cli" {
			for ui := range s.usersList {
				u := &s.usersList[ui]
				if s.rng.Float64() > 0.2+0.4*u.activity {
					continue
				}
				// Votes skew to the top of the ranking.
				pick := ranked[int(float64(len(ranked))*math.Pow(s.rng.Float64(), 2.2))]
				if pick.user == u.id {
					continue
				}
				voteRows = append(voteRows, []any{pick.id, u.id, w.slug, voteAt()})
			}
		} else {
			for ui := range s.usersList {
				u := &s.usersList[ui]
				if s.rng.Float64() > 0.1+0.35*u.activity {
					continue
				}
				seen := map[string]bool{}
				for k := 0; k < 6+s.rng.Intn(20); k++ {
					a, b := ranked[s.rng.Intn(len(ranked))], ranked[s.rng.Intn(len(ranked))]
					if a.id == b.id || a.user == u.id || b.user == u.id {
						continue
					}
					if a.id > b.id {
						a, b = b, a
					}
					if seen[a.id+b.id] {
						continue
					}
					seen[a.id+b.id] = true
					winner := "tie"
					if s.rng.Float64() > 0.1 {
						winner = "b"
						if s.rng.Float64() < 1/(1+math.Exp(-6*(a.quality-b.quality))) {
							winner = "a"
						}
					}
					t := voteAt()
					judgeRows = append(judgeRows, []any{w.slug, u.id, a.id, b.id, winner, t, t})
				}
			}
		}
	}
	if err := s.copy(ctx, tx, "product_entries", []string{"id", "task_slug", "user_id", "zip", "made_with", "status", "passed", "total", "failure_reason",
		"results", "log_tail", "created_at", "finished_at", "bench_ms", "bench_spread_ms"}, entryRows); err != nil {
		return err
	}
	if err := s.copy(ctx, tx, "product_votes", []string{"entry_id", "user_id", "task_slug", "created_at"}, voteRows); err != nil {
		return err
	}
	s.counts["product_votes"] = len(voteRows)
	s.counts["product_judgments"] = len(judgeRows)
	return s.copy(ctx, tx, "product_judgments", []string{"task_slug", "user_id", "entry_a", "entry_b", "winner", "created_at", "updated_at"}, judgeRows)
}
