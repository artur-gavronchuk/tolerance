package seed_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/fixtures/seed"
	"tolerance/internal/competitions"
	"tolerance/internal/platform/dbtest"
)

func TestTaskCompetitionInput_IsValid(t *testing.T) {
	now := time.Now()
	in, err := seed.TaskCompetitionInput("city-day-planner", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := competitions.Validate(in, now); err != nil {
		t.Fatalf("the seeded competition must pass the same validation as an admin-created one: %v", err)
	}
	weights := map[string]int{}
	sources := map[string]string{}
	for _, c := range in.Criteria {
		weights[c.Name], sources[c.Name] = c.Weight, c.Source
	}
	want := map[string]int{"Functionality": 60, "UX & polish": 20, "Code quality": 10, "Creativity": 10}
	for name, w := range want {
		if weights[name] != w {
			t.Errorf("%s weight = %d, want %d", name, weights[name], w)
		}
	}
	if sources["Functionality"] != "checks" || sources["UX & polish"] != "llm" {
		t.Errorf("sources: %v", sources)
	}
	if in.Points != 500 || in.Category != "Full build" || in.Difficulty != "Medium" {
		t.Errorf("unexpected header fields: %+v", in)
	}
}

func TestLoadTask_CreatesDraftOnceAndOnlyTheCompetition(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	created, err := seed.LoadTask(ctx, d.AdminPool, "city-day-planner", time.Now())
	if err != nil || !created {
		t.Fatalf("first load: created=%v err=%v", created, err)
	}
	created, err = seed.LoadTask(ctx, d.AdminPool, "city-day-planner", time.Now())
	if err != nil || created {
		t.Fatalf("second load must be a no-op: created=%v err=%v", created, err)
	}
	var comps, agents, subs int
	var status, suite string
	err = d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM competitions`).Scan(&comps); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM agents`).Scan(&agents); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM submissions`).Scan(&subs); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT status, check_suite FROM competitions WHERE slug = 'city-day-planner'`).Scan(&status, &suite)
	})
	if err != nil {
		t.Fatal(err)
	}
	if comps != 1 || agents != 0 || subs != 0 || status != "draft" || suite != "city-day-planner" {
		t.Fatalf("comps=%d agents=%d subs=%d status=%s suite=%s", comps, agents, subs, status, suite)
	}
}
