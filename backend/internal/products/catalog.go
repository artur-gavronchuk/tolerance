package products

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
	Days     int    `json:"days"`  // unused: every task runs for a calendar week
	Order    int    `json:"order"` // optional: lower goes first when the rotation picks the next task
}

// Sync loads <dir>/<slug>/{manifest.json,TASK.md,scenarios.json} and upserts them. A task's window (opens_at,
// deadline) is never set here: the weekly rotation (rotation.go) picks one task at a time; a task that left the directory is
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
		bench     []byte
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
		if l.m.Slug == "" || l.m.Slug != filepath.Base(base) {
			return 0, fmt.Errorf("products: %s: slug must equal the directory", mf)
		}
		switch l.m.Kind {
		case KindCLI:
			if l.m.Image == "" || l.m.Command == "" || l.m.TimeoutS <= 0 {
				return 0, fmt.Errorf("products: %s: a cli task needs image, command and timeout_s", mf)
			}
		case KindSite:
			// A site is judged by votes; it may also list Playwright scenarios (scenarios.json) that run in `image`.
		default:
			return 0, fmt.Errorf("products: %s: kind must be cli or site", mf)
		}
		md, err := os.ReadFile(filepath.Join(base, "TASK.md"))
		if err != nil {
			return 0, err
		}
		l.md = string(md)
		l.scenarios, err = os.ReadFile(filepath.Join(base, "scenarios.json"))
		if l.m.Kind == KindSite && os.IsNotExist(err) {
			l.scenarios = []byte("[]")
			all = append(all, l)
			continue
		}
		if err != nil {
			return 0, err
		}
		if l.m.Kind == KindSite && (l.m.Image == "" || l.m.TimeoutS <= 0) {
			return 0, fmt.Errorf("products: %s: a site task with scenarios needs image and timeout_s", base)
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
		if l.m.Kind == KindCLI {
			if l.bench, err = loadBench(base); err != nil {
				return 0, err
			}
		}
		all = append(all, l)
	}
	slugs := make([]string, 0, len(all))
	err = pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, l := range all {
			if _, err := tx.Exec(ctx, `
				INSERT INTO product_tasks (slug, title, summary, kind, task_md, image, command, timeout_s, scenarios, ord, bench)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
				ON CONFLICT (slug) DO UPDATE SET title = $2, summary = $3, kind = $4, task_md = $5, image = $6, command = $7,
				    timeout_s = $8, scenarios = $9, ord = $10, bench = $11, active = true, synced_at = now()`,
				l.m.Slug, l.m.Title, l.m.Summary, l.m.Kind, l.md, l.m.Image, l.m.Command, l.m.TimeoutS, l.scenarios, l.m.Order, l.bench); err != nil {
				return fmt.Errorf("products: sync %s: %w", l.m.Slug, err)
			}
			slugs = append(slugs, l.m.Slug)
		}
		_, err := tx.Exec(ctx, `UPDATE product_tasks SET active = false WHERE active AND NOT frozen AND NOT (slug = ANY($1))`, slugs)
		return err
	})
	return len(all), err
}

// benchDef is bench.json: how many timed passes to take. The generator (bench_gen.py, see runner.py) ships with it.
type benchDef struct {
	Runs int `json:"runs"`
}

// loadBench reads <dir>/bench.json and bench_gen.py into the product_tasks.bench document; nil when the task has no
// benchmark.
func loadBench(dir string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "bench.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var d benchDef
	if err := json.Unmarshal(raw, &d); err != nil || d.Runs < 1 || d.Runs > 20 {
		return nil, fmt.Errorf("products: %s: bench.json needs runs between 1 and 20", dir)
	}
	gen, err := os.ReadFile(filepath.Join(dir, "bench_gen.py"))
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"runs": d.Runs, "gen": string(gen)})
}
