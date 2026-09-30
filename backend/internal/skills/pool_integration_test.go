package skills_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/skills"
)

func mustExec(t *testing.T, d *dbtest.DB, sql string, args ...any) {
	t.Helper()
	err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
	if err != nil {
		t.Fatalf("seed (%s): %v", sql, err)
	}
}

func seedSkillTask(t *testing.T, d *dbtest.DB, skill, slug string, challengeOnly bool) {
	t.Helper()
	mustExec(t, d, `INSERT INTO skills (slug, title, language, image, run_cmd)
		VALUES ($1, $1, 'go', 'arena-skill-go:1', 'go test -json ./...') ON CONFLICT DO NOTHING`, skill)
	mustExec(t, d, `INSERT INTO skill_tasks
		(slug, skill_slug, title, difficulty, agent_timeout_s, sandbox_timeout_s, hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256, challenge_only)
		VALUES ($1, $2, $1, 1, 600, 120, 4, '# t', '\x00', '\x00', 'sha', $3)`, slug, skill, challengeOnly)
}

func seedAgent(t *testing.T, d *dbtest.DB) string {
	t.Helper()
	userID, agentID := idgen.New("user"), idgen.New("agent")
	mustExec(t, d, `INSERT INTO users (id, email) VALUES ($1, $1 || '@example.com')`, userID)
	mustExec(t, d, `INSERT INTO agents (id, owner_user_id, name) VALUES ($1, $2, $1)`, agentID, userID)
	return agentID
}

// record runs RecordExposure as the worker role, which is what queues a run's
// next task in production, so its grants are exercised too.
func record(t *testing.T, d *dbtest.DB, slug, agentID string) bool {
	t.Helper()
	var first bool
	err := d.WorkerPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		first, err = skills.RecordExposure(ctx, tx, slug, agentID)
		return err
	})
	if err != nil {
		t.Fatalf("record exposure: %v", err)
	}
	return first
}

func exposures(t *testing.T, d *dbtest.DB, slug string) (int, bool) {
	t.Helper()
	var n int
	var retired *string
	err := d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT exposures, retired_at::text FROM skill_tasks WHERE slug = $1`, slug).Scan(&n, &retired)
	})
	if err != nil {
		t.Fatalf("read exposures: %v", err)
	}
	return n, retired != nil
}

func pool(t *testing.T, d *dbtest.DB, skill string) []string {
	t.Helper()
	var out []string
	err := d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = skills.Pool(ctx, tx, skill)
		return err
	})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	return out
}

func TestRecordExposureCountsDistinctAgents(t *testing.T) {
	d := dbtest.New(t)
	seedSkillTask(t, d, "go", "lru-cache-eviction", false)
	a1, a2 := seedAgent(t, d), seedAgent(t, d)

	if !record(t, d, "lru-cache-eviction", a1) {
		t.Error("the first sighting must report first = true")
	}
	for i := 0; i < 2; i++ {
		if record(t, d, "lru-cache-eviction", a1) {
			t.Error("re-issuing the same task to the same agent must report first = false")
		}
	}
	if n, _ := exposures(t, d, "lru-cache-eviction"); n != 1 {
		t.Fatalf("after three handouts to one agent: exposures = %d, want 1", n)
	}

	if !record(t, d, "lru-cache-eviction", a2) {
		t.Error("a second agent's first sighting must report first = true")
	}
	if n, _ := exposures(t, d, "lru-cache-eviction"); n != 2 {
		t.Fatalf("after a second agent: exposures = %d, want 2", n)
	}
}

func TestRecordExposureRetiresAtTheLimit(t *testing.T) {
	d := dbtest.New(t)
	seedSkillTask(t, d, "go", "worker-pool-shutdown", false)
	for i := 0; i < skills.MaxExposures; i++ {
		record(t, d, "worker-pool-shutdown", seedAgent(t, d))
		n, retired := exposures(t, d, "worker-pool-shutdown")
		wantRetired := i+1 >= skills.MaxExposures
		if n != i+1 || retired != wantRetired {
			t.Fatalf("after %d agents: exposures = %d retired = %v, want %d %v", i+1, n, retired, i+1, wantRetired)
		}
	}
	if got := pool(t, d, "go"); len(got) != 0 {
		t.Fatalf("pool = %v, want empty after the only task retired", got)
	}
}

func TestPoolExcludesRetiredInactiveAndChallengeOnly(t *testing.T) {
	d := dbtest.New(t)
	seedSkillTask(t, d, "go", "keep", false)
	seedSkillTask(t, d, "go", "challenge-task", true)
	seedSkillTask(t, d, "go", "retired", false)
	seedSkillTask(t, d, "go", "inactive", false)
	mustExec(t, d, `UPDATE skill_tasks SET retired_at = now() WHERE slug = 'retired'`)
	mustExec(t, d, `UPDATE skill_tasks SET active = false WHERE slug = 'inactive'`)
	if got := pool(t, d, "go"); fmt.Sprint(got) != "[keep]" {
		t.Fatalf("pool = %v, want [keep]", got)
	}
}
