package skills_test

import (
	"context"
	"testing"

	"tolerance/internal/platform/dbtest"
	"tolerance/internal/skills"
)

func TestSyncCatalog_DeactivatesTasksGoneFromCatalog(t *testing.T) {
	env := dbtest.New(t)
	ctx := context.Background()
	sk := []skills.Skill{{Slug: "go", Title: "Go", Language: "go", Image: "arena-skill-go:1", RunCmd: "go test ./..."}}
	mk := func(slug string) skills.Task {
		return skills.Task{Slug: slug, SkillSlug: "go", Title: slug, Difficulty: 1, AgentTimeoutS: 60, SandboxTimeoutS: 60,
			HiddenTests: 1, TaskMD: "x", RepoTar: []byte("r"), HiddenTar: []byte("h"), RepoSHA256: "sha"}
	}
	if err := skills.SyncCatalog(ctx, env.AdminPool, sk, []skills.Task{mk("t1"), mk("t2")}); err != nil {
		t.Fatal(err)
	}
	if err := skills.SyncCatalog(ctx, env.AdminPool, sk, []skills.Task{mk("t1")}); err != nil {
		t.Fatal(err)
	}
	active := map[string]bool{}
	rows, err := env.AppPool.Raw().Query(ctx, `SELECT slug, active FROM skill_tasks`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		var a bool
		if err := rows.Scan(&slug, &a); err != nil {
			t.Fatal(err)
		}
		active[slug] = a
	}
	if len(active) != 2 || !active["t1"] || active["t2"] {
		t.Fatalf("active flags: %v", active)
	}
	// A task that returns to the catalog is picked again.
	if err := skills.SyncCatalog(ctx, env.AdminPool, sk, []skills.Task{mk("t1"), mk("t2")}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := env.AppPool.Raw().QueryRow(ctx, `SELECT count(*) FROM skill_tasks WHERE active`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("reactivated: %d %v", n, err)
	}
}
