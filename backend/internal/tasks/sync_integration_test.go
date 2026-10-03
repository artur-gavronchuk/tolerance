package tasks_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/dbtest"
	"tolerance/internal/tasks"
)

// A played task (one that has been a daily task) is frozen: a sync that changes what it is checked
// against is rejected as a whole, while presentation fields and unplayed tasks may still change.
func TestSyncFreezesPlayedTasks(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	ts, err := tasks.LoadByLanguage(filepath.Join("..", "..", "fixtures", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) < 2 {
		t.Fatalf("need two fixture tasks, have %d", len(ts))
	}
	if err := tasks.Sync(ctx, d.AdminPool, ts); err != nil {
		t.Fatal(err)
	}
	played, unplayed := ts[0], ts[1]
	if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO daily_tasks (day, task_slug) VALUES ('2026-01-01', $1)`, played.Slug)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	hiddenTar := func(slug string) []byte {
		t.Helper()
		var b []byte
		if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT hidden_tar FROM tasks WHERE slug = $1`, slug).Scan(&b)
		}); err != nil {
			t.Fatal(err)
		}
		return b
	}
	with := func(i int, change func(*tasks.Task)) []tasks.Task {
		out := append([]tasks.Task(nil), ts...)
		change(&out[i])
		return out
	}

	// Unchanged catalog: fine.
	if err := tasks.Sync(ctx, d.AdminPool, ts); err != nil {
		t.Fatalf("resync of an unchanged catalog: %v", err)
	}
	// Presentation of a played task may change.
	if err := tasks.Sync(ctx, d.AdminPool, with(0, func(x *tasks.Task) { x.Title += " (renamed)" })); err != nil {
		t.Fatalf("title change on a played task: %v", err)
	}
	// What a played task is checked against may not, and nothing of that sync is applied.
	before := hiddenTar(played.Slug)
	changed := with(0, func(x *tasks.Task) { x.HiddenTar = append(append([]byte(nil), x.HiddenTar...), 0) })
	changed[1].Title += " (should not land)"
	err = tasks.Sync(ctx, d.AdminPool, changed)
	if err == nil || !strings.Contains(err.Error(), played.Slug) {
		t.Fatalf("hidden tests of a played task changed: want an error naming %s, got %v", played.Slug, err)
	}
	if string(hiddenTar(played.Slug)) != string(before) {
		t.Fatal("rejected sync still rewrote the played task's hidden tests")
	}
	var title string
	if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT title FROM tasks WHERE slug = $1`, unplayed.Slug).Scan(&title)
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(title, "should not land") {
		t.Fatal("rejected sync was partly applied")
	}
	// A run parameter counts too.
	if err := tasks.Sync(ctx, d.AdminPool, with(0, func(x *tasks.Task) { x.SandboxTimeoutS++ })); err == nil {
		t.Fatal("timeout change on a played task was accepted")
	}
	// An unplayed task may be edited freely.
	if err := tasks.Sync(ctx, d.AdminPool, with(1, func(x *tasks.Task) { x.TaskMD += "\nClarified." })); err != nil {
		t.Fatalf("edit of an unplayed task: %v", err)
	}
}
