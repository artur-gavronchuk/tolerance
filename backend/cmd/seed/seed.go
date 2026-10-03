package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type seedUser struct {
	id, email, handle string
	created           time.Time
	skill             float64 // 0..1, how likely the agent gets a task right
	activity          float64 // chance to show up on a given day
	madeWith          []string
	botID             string
}

type seeder struct {
	rng       *rand.Rand
	n         int
	now       time.Time
	today     time.Time // 00:00 UTC
	usersList []seedUser
	counts    map[string]int
}

func newSeeder(n int) *seeder {
	now := time.Now().UTC()
	return &seeder{
		rng:    rand.New(rand.NewSource(20261003)),
		n:      n,
		now:    now,
		today:  time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC),
		counts: map[string]int{},
	}
}

const seedUsersSQL = `(SELECT id FROM users WHERE email LIKE '%` + emailDomain + `')`
const seedBotsSQL = `(SELECT id FROM game_bots WHERE owner_user_id IN ` + seedUsersSQL + `)`

func (s *seeder) copy(ctx context.Context, tx pgx.Tx, table string, cols []string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{table}, cols, pgx.CopyFromRows(rows))
	return err
}

func (s *seeder) cleanup(ctx context.Context, tx pgx.Tx) error {
	stmts := []string{
		`DELETE FROM notifications WHERE user_id IN ` + seedUsersSQL,
		`DELETE FROM notify_state WHERE user_id IN ` + seedUsersSQL,
		`DELETE FROM upload_links WHERE user_id IN ` + seedUsersSQL,
		`DELETE FROM submissions WHERE user_id IN ` + seedUsersSQL,
		// Tanks: tournaments and matches that mention a seeded bot (or carry the seed prefix) go first.
		`CREATE TEMP TABLE seed_tournaments ON COMMIT DROP AS SELECT id FROM tanks_tournaments
			WHERE id LIKE '` + idPrefix + `%' OR id IN (SELECT tournament_id FROM tanks_tournament_entries WHERE bot_id IN ` + seedBotsSQL + `)`,
		`CREATE TEMP TABLE seed_matches ON COMMIT DROP AS SELECT id FROM matches
			WHERE id LIKE '` + idPrefix + `%' OR id IN (SELECT match_id FROM match_players WHERE bot_id IN ` + seedBotsSQL + `)`,
		`DELETE FROM tanks_tournament_games WHERE pairing_id IN (SELECT id FROM tanks_tournament_pairings WHERE tournament_id IN (SELECT id FROM seed_tournaments))
			OR match_id IN (SELECT id FROM seed_matches)`,
		`DELETE FROM tanks_tournament_pairings WHERE tournament_id IN (SELECT id FROM seed_tournaments)`,
		`DELETE FROM tanks_tournament_entries WHERE tournament_id IN (SELECT id FROM seed_tournaments) OR bot_id IN ` + seedBotsSQL,
		`DELETE FROM tanks_tournaments WHERE id IN (SELECT id FROM seed_tournaments)`,
		`DELETE FROM tanks_broadcasts WHERE match_id IN (SELECT id FROM seed_matches)`,
		`DELETE FROM matches WHERE id IN (SELECT id FROM seed_matches)`,
		`DELETE FROM jobs WHERE kind = 'run_match' AND state IN ('queued', 'leased')
			AND NOT EXISTS (SELECT 1 FROM matches m WHERE m.id = jobs.payload ->> 'match_id')`,
		`DELETE FROM tanks_season_standings WHERE season_id IN (SELECT season_id FROM tanks_season_standings WHERE bot_id IN ` + seedBotsSQL + `)`,
		`DELETE FROM tanks_season_ratings WHERE bot_id IN ` + seedBotsSQL,
		`UPDATE game_bots SET active_version_id = NULL WHERE owner_user_id IN ` + seedUsersSQL,
		`DELETE FROM bot_versions WHERE bot_id IN ` + seedBotsSQL,
		`DELETE FROM game_bots WHERE owner_user_id IN ` + seedUsersSQL,
		`DELETE FROM tanks_seasons s WHERE s.status = 'archived'
			AND NOT EXISTS (SELECT 1 FROM tanks_season_standings x WHERE x.season_id = s.id)
			AND NOT EXISTS (SELECT 1 FROM tanks_season_ratings x WHERE x.season_id = s.id)
			AND NOT EXISTS (SELECT 1 FROM tanks_tournaments x WHERE x.season_id = s.id)`,
		`DELETE FROM users WHERE email LIKE '%` + emailDomain + `'`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(ctx, q); err != nil {
			return fmt.Errorf("%s: %w", strings.Fields(q)[0]+" "+strings.Fields(q)[2], err)
		}
	}
	return nil
}

var (
	firstWords = []string{"quiet", "swift", "lazy", "bright", "rusty", "lucky", "sunny", "grim", "tiny", "hasty", "calm", "bold", "dusty", "witty", "keen", "odd", "wild", "slick", "mellow", "frosty", "amber", "cosmic", "pixel", "turbo", "neon", "paper", "iron", "velvet", "ghost", "solar"}
	lastWords  = []string{"otter", "falcon", "badger", "lynx", "heron", "panda", "gecko", "raven", "moose", "newt", "orca", "wombat", "fox", "ibex", "koala", "mantis", "yak", "marten", "crane", "bison", "stoat", "shrike", "tapir", "viper", "weasel"}
)

type stackChoice struct {
	label  string
	weight int
	bonus  float64
}

var stackChoices = []stackChoice{
	{"Claude Code + Opus 4.5", 14, 0.10},
	{"Claude Code (Sonnet 4.5)", 14, 0.05},
	{"claude code haiku 4.5", 4, -0.08},
	{"Claude Code Opus", 6, 0.10},
	{"Codex CLI + GPT-5", 12, 0.07},
	{"codex gpt-5", 4, 0.07},
	{"Cursor", 8, 0},
	{"Cursor + Sonnet 4.5", 5, 0.05},
	{"Aider + GPT-5", 5, 0.04},
	{"aider gpt-4.1", 3, -0.05},
	{"Gemini CLI", 6, 0.02},
	{"Gemini CLI (Gemini 2.5 Pro)", 3, 0.03},
	{"Cline + DeepSeek", 3, -0.03},
	{"Windsurf", 3, -0.02},
	{"Copilot agent", 4, -0.04},
	{"OpenHands + Qwen", 2, -0.06},
	{"Custom agent", 3, 0},
	{"", 5, -0.02},
}

func (s *seeder) pickStack() stackChoice {
	total := 0
	for _, c := range stackChoices {
		total += c.weight
	}
	r := s.rng.Intn(total)
	for _, c := range stackChoices {
		if r < c.weight {
			return c
		}
		r -= c.weight
	}
	return stackChoices[0]
}

func stackBonus(label string) float64 {
	for _, c := range stackChoices {
		if c.label == label {
			return c.bonus
		}
	}
	return 0
}

func (s *seeder) users(ctx context.Context, tx pgx.Tx) error {
	used := map[string]bool{}
	var rows, idents [][]any
	for i := 0; i < s.n; i++ {
		var handle string
		for {
			handle = firstWords[s.rng.Intn(len(firstWords))] + "-" + lastWords[s.rng.Intn(len(lastWords))]
			if s.rng.Intn(3) == 0 {
				handle += fmt.Sprint(s.rng.Intn(100))
			}
			if !used[strings.ToLower(handle)] {
				used[strings.ToLower(handle)] = true
				break
			}
			handle += fmt.Sprint(i)
			if !used[strings.ToLower(handle)] {
				used[strings.ToLower(handle)] = true
				break
			}
		}
		u := seedUser{
			id:      fmt.Sprintf("%su%04d", idPrefix, i+1),
			email:   fmt.Sprintf("seed-%04d%s", i+1, emailDomain),
			handle:  handle,
			created: s.now.Add(-time.Duration(s.rng.Intn(45*24*60)) * time.Minute),
			skill:   math.Min(0.95, math.Max(0.1, 0.55+0.22*s.rng.NormFloat64())),
		}
		switch r := s.rng.Float64(); {
		case i < 12 || r < 0.08: // regulars, the streak holders
			u.activity = 0.9
			u.created = s.now.Add(-time.Duration(32+s.rng.Intn(20)) * 24 * time.Hour)
		case r < 0.35:
			u.activity = 0.45
		default:
			u.activity = 0.1
		}
		u.madeWith = []string{s.pickStack().label}
		if s.rng.Intn(3) == 0 {
			u.madeWith = append(u.madeWith, s.pickStack().label)
		}
		s.usersList = append(s.usersList, u)
		rows = append(rows, []any{u.id, u.email, "user", u.created, u.handle})
		idents = append(idents, []any{"dev", u.email, u.id, u.email, u.handle, u.created, u.created})
	}
	s.counts["users"] = len(rows)
	if err := s.copy(ctx, tx, "users", []string{"id", "email", "role", "created_at", "handle"}, rows); err != nil {
		return err
	}
	return s.copy(ctx, tx, "user_identities", []string{"provider", "subject", "user_id", "email", "login", "created_at", "last_login_at"}, idents)
}

type dailyTask struct {
	slug   string
	title  string
	tests  int
	diff   int
	kind   string
	cases  int
	dir    *string
	valid  bool
	scored bool
}

const sampleDiff = "diff --git a/fix.txt b/fix.txt\n--- a/fix.txt\n+++ b/fix.txt\n@@ -1,3 +1,3 @@\n context line\n-the broken behaviour\n+the fixed behaviour\n context line\n"

func (s *seeder) daily(ctx context.Context, tx pgx.Tx) error {
	var slugs []string
	tasks := map[string]dailyTask{}
	rows, err := tx.Query(ctx, `SELECT slug, title, hidden_tests, difficulty, kind, cases, direction FROM tasks WHERE active ORDER BY slug`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var t dailyTask
		if err := rows.Scan(&t.slug, &t.title, &t.tests, &t.diff, &t.kind, &t.cases, &t.dir); err != nil {
			rows.Close()
			return err
		}
		tasks[t.slug] = t
		slugs = append(slugs, t.slug)
	}
	rows.Close()
	if len(slugs) == 0 {
		return fmt.Errorf("no active tasks: run make migrate first")
	}
	// A task per day, 29 days back to today; days that already have one keep it.
	for d := 29; d >= 0; d-- {
		day := s.today.AddDate(0, 0, -d)
		if _, err := tx.Exec(ctx, `INSERT INTO daily_tasks (day, task_slug) VALUES ($1, $2) ON CONFLICT (day) DO NOTHING`,
			day, slugs[(29-d+7)%len(slugs)]); err != nil {
			return err
		}
	}
	dayTask := map[time.Time]dailyTask{}
	drows, err := tx.Query(ctx, `SELECT day, task_slug FROM daily_tasks WHERE day >= $1`, s.today.AddDate(0, 0, -29))
	if err != nil {
		return err
	}
	for drows.Next() {
		var day time.Time
		var slug string
		if err := drows.Scan(&day, &slug); err != nil {
			drows.Close()
			return err
		}
		dayTask[time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)] = tasks[slug]
	}
	drows.Close()

	var subs [][]any
	n := 0
	for d := 29; d >= 0; d-- {
		day := s.today.AddDate(0, 0, -d)
		t, ok := dayTask[day]
		if !ok {
			continue
		}
		weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
		for ui := range s.usersList {
			u := &s.usersList[ui]
			if u.created.After(day.Add(24 * time.Hour)) {
				continue
			}
			p := u.activity
			if weekend {
				p *= 0.7
			}
			if s.rng.Float64() > p {
				continue
			}
			stack := u.madeWith[s.rng.Intn(len(u.madeWith))]
			base := day.Add(time.Duration(s.rng.Intn(24*60-30)) * time.Minute)
			if d == 0 {
				span := int(s.now.Sub(day).Minutes()) - 5
				if span < 1 {
					continue
				}
				base = day.Add(time.Duration(s.rng.Intn(span)) * time.Minute)
			}
			pPass := u.skill + stackBonus(stack) - 0.12*float64(t.diff-1)
			for k := 0; k < 3; k++ {
				at := base.Add(time.Duration(k*(3+s.rng.Intn(20))) * time.Minute)
				if at.After(s.now.Add(-time.Minute)) {
					break
				}
				status, passed, reason := "failed", 0, ptr("hidden_tests_failed")
				switch r := s.rng.Float64(); {
				case r < 0.015:
					status, reason = "infra_error", ptr("sandbox_unavailable")
				case r < 0.03:
					reason = ptr("diff_does_not_apply")
				case s.rng.Float64() < pPass+0.1*float64(k):
					status, passed, reason = "passed", t.tests, nil
				default:
					passed = s.rng.Intn(t.tests)
					if s.rng.Intn(4) == 0 {
						passed = 0
					}
				}
				var tj strings.Builder
				tj.WriteString("[")
				for i := 0; i < t.tests; i++ {
					if i > 0 {
						tj.WriteString(",")
					}
					fmt.Fprintf(&tj, `{"name":"test_case_%d","passed":%t}`, i+1, i < passed)
				}
				tj.WriteString("]")
				total := t.tests
				if status == "infra_error" {
					passed, total = 0, 0
					tj.Reset()
					tj.WriteString("[]")
				}
				id := fmt.Sprintf("%ss%d", idPrefix, n)
				n++
				subs = append(subs, []any{id, u.id, t.slug, day, sampleDiff, stack, status, passed, total, reason, tj.String(), "", at, at.Add(time.Duration(15+s.rng.Intn(60)) * time.Second)})
				if status == "passed" {
					break
				}
			}
		}
	}
	s.counts["submissions"] = len(subs)
	return s.copy(ctx, tx, "submissions", []string{"id", "user_id", "task_slug", "day", "diff", "made_with", "status", "passed_tests", "total_tests",
		"failure_reason", "tests", "log_tail", "created_at", "finished_at"}, subs)
}

func ptr[T any](v T) *T { return &v }
