package products

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
)

type manifest struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Kind     string `json:"kind"`
	Image    string `json:"image"`
	Command  string `json:"command"`
	TimeoutS int    `json:"timeout_s"`
	Days     int    `json:"days"`
}

// Sync loads <dir>/<slug>/{manifest.json,TASK.md,scenarios.json} and upserts them. A task's window (opens_at,
// deadline) is set when it first appears and kept on later syncs; a task that left the directory is
// deactivated, its entries stay.
func Sync(ctx context.Context, pool *db.Pool, dir string) (int, error) {
	dirs, err := filepath.Glob(filepath.Join(dir, "*", "manifest.json"))
	if err != nil {
		return 0, err
	}
	if len(dirs) == 0 {
		return 0, nil // an unmounted catalog must not deactivate everything
	}
	type loaded struct {
		m         manifest
		md        string
		scenarios []byte
	}
	var all []loaded
	for _, mf := range dirs {
		base := filepath.Dir(mf)
		var l loaded
		raw, err := os.ReadFile(mf)
		if err != nil {
			return 0, err
		}
		if err := json.Unmarshal(raw, &l.m); err != nil {
			return 0, fmt.Errorf("products: %s: %w", mf, err)
		}
		if l.m.Slug == "" || l.m.Slug != filepath.Base(base) || l.m.Kind != "cli" || l.m.Image == "" || l.m.Command == "" || l.m.TimeoutS <= 0 {
			return 0, fmt.Errorf("products: %s: slug (must equal the directory), kind=cli, image, command and timeout_s are required", mf)
		}
		if l.m.Days <= 0 {
			l.m.Days = 7
		}
		md, err := os.ReadFile(filepath.Join(base, "TASK.md"))
		if err != nil {
			return 0, err
		}
		l.md = string(md)
		if l.scenarios, err = os.ReadFile(filepath.Join(base, "scenarios.json")); err != nil {
			return 0, err
		}
		var scs []Scenario
		if err := json.Unmarshal(l.scenarios, &scs); err != nil || len(scs) == 0 {
			return 0, fmt.Errorf("products: %s: scenarios.json must be a non-empty list of scenarios", base)
		}
		for _, sc := range scs {
			if sc.Name == "" {
				return 0, fmt.Errorf("products: %s: every scenario needs a name", base)
			}
		}
		all = append(all, l)
	}
	slugs := make([]string, 0, len(all))
	err = pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, l := range all {
			if _, err := tx.Exec(ctx, `
				INSERT INTO product_tasks (slug, title, summary, kind, task_md, image, command, timeout_s, scenarios, deadline)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
				ON CONFLICT (slug) DO UPDATE SET title = $2, summary = $3, kind = $4, task_md = $5, image = $6, command = $7,
				    timeout_s = $8, scenarios = $9, active = true, synced_at = now()`,
				l.m.Slug, l.m.Title, l.m.Summary, l.m.Kind, l.md, l.m.Image, l.m.Command, l.m.TimeoutS, l.scenarios,
				time.Now().UTC().Add(time.Duration(l.m.Days)*24*time.Hour)); err != nil {
				return fmt.Errorf("products: sync %s: %w", l.m.Slug, err)
			}
			slugs = append(slugs, l.m.Slug)
		}
		_, err := tx.Exec(ctx, `UPDATE product_tasks SET active = false WHERE active AND NOT (slug = ANY($1))`, slugs)
		return err
	})
	return len(all), err
}
