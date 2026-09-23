package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/fixtures/tasks"
	"tolerance/internal/competitions"
	"tolerance/internal/platform/db"
)

// taskOwnerID is the account that owns seeded task competitions. It never
// signs in: it exists so created_by has a valid owner.
const taskOwnerID = "user_seed_arena"

// TaskCompetitionInput builds the competition for a published task bundle:
// Functionality is measured by the automated checks, the rest is rated
// qualitatively. The deadline is two weeks out; the competition is created
// as a draft and published by an admin.
func TaskCompetitionInput(slug string, now time.Time) (competitions.Input, error) {
	if slug != "city-day-planner" {
		return competitions.Input{}, fmt.Errorf("seed: no competition definition for task %q", slug)
	}
	b, err := tasks.Load(slug)
	if err != nil {
		return competitions.Input{}, err
	}
	var task struct {
		WhatToBuild string `json:"what_to_build"`
	}
	if err := json.Unmarshal(b.Task, &task); err != nil {
		return competitions.Input{}, err
	}
	return competitions.Input{
		Slug:  slug,
		Title: "Day planner for an unfamiliar city",
		Summary: "Build a small web app that plans one day out in a city you have never visited, " +
			"from a fixed set of 24 places, within a time window and a budget.",
		Brief:                task.WhatToBuild,
		Category:             "Full build",
		Difficulty:           "Medium",
		Points:               500,
		Deadline:             now.Add(14 * 24 * time.Hour).UTC(),
		MatchDurationSeconds: 900,
		Task:                 b.Task,
		CheckSuite:           b.Suite,
		Criteria: []competitions.Criterion{
			{Name: "Functionality", Weight: 60, Source: competitions.SourceChecks,
				Description: "Share of the eight automated browser checks passed against your live app."},
			{Name: "UX & polish", Weight: 20, Source: competitions.SourceLLM,
				Description: "Clarity, responsiveness (including 375px) and visual quality, judged from recorded screenshots."},
			{Name: "Code quality", Weight: 10, Source: competitions.SourceLLM,
				Description: "Structure, readability and maintainability; rated only when public source is available at the given commit."},
			{Name: "Creativity", Weight: 10, Source: competitions.SourceLLM,
				Description: "Thoughtful touches beyond the brief."},
		},
	}, nil
}

// LoadTask creates the draft competition for a task bundle. It creates
// nothing but the competition (plus its owner account) and is a no-op when
// the slug already exists, so it is safe to run on any database.
func LoadTask(ctx context.Context, pool *db.Pool, slug string, now time.Time) (created bool, err error) {
	in, err := TaskCompetitionInput(slug, now)
	if err != nil {
		return false, err
	}
	if err := competitions.Validate(in, now); err != nil {
		return false, fmt.Errorf("seed: task competition %q is invalid: %w", slug, err)
	}
	criteria, err := json.Marshal(in.Criteria)
	if err != nil {
		return false, err
	}
	err = pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM competitions WHERE slug = $1)`, slug).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name)
			VALUES ($1, 'seed', 'arena-seed', 'arena-seed', 'Agent Arena') ON CONFLICT DO NOTHING`, taskOwnerID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO competitions (id, slug, title, summary, brief, category, difficulty, status, points, deadline,
				match_duration_seconds, criteria, created_by, task, check_suite)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'draft',$8,$9,$10,$11,$12,$13,$14)`,
			"comp_seed_"+slug, slug, in.Title, in.Summary, in.Brief, in.Category, in.Difficulty, in.Points, in.Deadline,
			in.MatchDurationSeconds, criteria, taskOwnerID, []byte(in.Task), in.CheckSuite); err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}
