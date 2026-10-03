package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/admin"
	"tolerance/internal/arena"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/skillrating"
)

type fx struct {
	d         *dbtest.DB
	svc       *admin.Service
	adminID   string
	agentID   string
	versionID string
	taskSlugs []string
	runIDs    []string
}

func (f *fx) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	err := f.d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
	if err != nil {
		t.Fatalf("seed (%s): %v", sql, err)
	}
}

// setup seeds an agent with n scored qualification runs on skill "go", each one
// a perfect score, and the rating those runs produce. It writes the rows
// directly: this package's job is what an administrator does to an existing
// rating, not how the rating was earned (that is internal/qualifications).
func setup(t *testing.T, n int) *fx {
	t.Helper()
	d := dbtest.New(t)
	f := &fx{d: d, svc: admin.NewService(d.AppPool)}

	f.adminID = idgen.New("user")
	f.exec(t, `INSERT INTO users (id, email, role) VALUES ($1, 'admin@example.com', 'admin')`, f.adminID)
	owner := idgen.New("user")
	f.exec(t, `INSERT INTO users (id, email) VALUES ($1, 'owner@example.com')`, owner)
	f.agentID = idgen.New("agent")
	f.exec(t, `INSERT INTO agents (id, owner_user_id, name) VALUES ($1, $2, 'fixer')`, f.agentID, owner)
	f.versionID = idgen.New("ver")
	f.exec(t, `INSERT INTO agent_versions (id, agent_id, number, model, harness, config_digest)
		VALUES ($1, $2, 1, 'claude-opus-5', 'claude-code 2.1', $1)`, f.versionID, f.agentID)
	f.exec(t, `UPDATE agents SET current_version_id = $2 WHERE id = $1`, f.agentID, f.versionID)
	f.exec(t, `INSERT INTO skills (slug, title, language, image, run_cmd)
		VALUES ('go', 'Go', 'go', 'arena-skill-go:1', 'go test -json ./...')`)
	for i := 0; i < 3; i++ {
		slug := idgen.New("task")
		f.exec(t, `INSERT INTO skill_tasks (slug, skill_slug, title, difficulty, agent_timeout_s, sandbox_timeout_s,
			hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256) VALUES ($1, 'go', $1, 1, 600, 120, 4, '# t', '\x00', '\x00', 'sha')`, slug)
		f.taskSlugs = append(f.taskSlugs, slug)
	}

	st := skillrating.State{Uncertainty: skillrating.MaxUncert}
	for i := 0; i < n; i++ {
		st = skillrating.Apply(st, 1)
		runID := idgen.New("qrun")
		f.exec(t, `INSERT INTO qualification_runs (id, agent_id, version_id, skill_slug, status, finished_at, score,
			rating_before, rating_after, uncertainty_after, task_slugs)
			VALUES ($1, $2, $3, 'go', 'scored', now(), 1.0, 1000, $4, $5, $6)`,
			runID, f.agentID, f.versionID, st.Rating, st.Uncertainty, f.taskSlugs)
		f.runIDs = append(f.runIDs, runID)
	}
	f.exec(t, `INSERT INTO skill_ratings (agent_id, skill_slug, version_id, rating, uncertainty, runs, sum_targets)
		VALUES ($1, 'go', $2, $3, $4, $5, $6)`, f.agentID, f.versionID, st.Rating, st.Uncertainty, st.Runs, st.SumTargets)
	return f
}

type rating struct {
	Rating, Uncertainty, Runs, SumTargets int
	Exists                                bool
}

func (f *fx) rating(t *testing.T) rating {
	t.Helper()
	var r rating
	err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT rating, uncertainty, runs, sum_targets FROM skill_ratings
			WHERE agent_id = $1 AND skill_slug = 'go'`, f.agentID).Scan(&r.Rating, &r.Uncertainty, &r.Runs, &r.SumTargets)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		r.Exists = err == nil
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *fx) runStatus(t *testing.T, id string) string {
	t.Helper()
	var st string
	err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT status FROM qualification_runs WHERE id = $1`, id).Scan(&st)
	})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func (f *fx) audited(t *testing.T, action, aggregateID string) bool {
	t.Helper()
	var n int
	err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action = $1 AND aggregate_id = $2 AND reason <> ''`,
			action, aggregateID).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func problem(t *testing.T, err error, status int, code string) {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) {
		t.Fatalf("expected a problem, got %v", err)
	}
	if p.Status != status || p.Code != code {
		t.Fatalf("got %d %s, want %d %s", p.Status, p.Code, status, code)
	}
}

func TestVoidRunSubtractsItFromTheRating(t *testing.T) {
	f := setup(t, 2)
	before := f.rating(t)

	if err := f.svc.VoidRun(context.Background(), f.adminID, f.runIDs[1], "a human in the agent log"); err != nil {
		t.Fatalf("void: %v", err)
	}

	after := f.rating(t)
	if after.Runs != before.Runs-1 {
		t.Fatalf("runs = %d, want %d", after.Runs, before.Runs-1)
	}
	if after.Uncertainty <= before.Uncertainty {
		t.Errorf("uncertainty = %d, want more than %d: removing evidence cannot raise confidence",
			after.Uncertainty, before.Uncertainty)
	}
	if st := f.runStatus(t, f.runIDs[1]); st != "voided" {
		t.Fatalf("run status = %q, want voided", st)
	}
	if !f.audited(t, "admin.run_voided", f.runIDs[1]) {
		t.Error("voiding must leave exactly one audit event carrying its reason")
	}
	// The other run is untouched.
	if st := f.runStatus(t, f.runIDs[0]); st != "scored" {
		t.Errorf("the other run = %q, want scored", st)
	}
}

func TestVoidRunRequiresAReason(t *testing.T) {
	f := setup(t, 1)
	err := f.svc.VoidRun(context.Background(), f.adminID, f.runIDs[0], "  ")
	problem(t, err, 422, "validation_failed")
}

func TestVoidRunTwiceIsRefused(t *testing.T) {
	f := setup(t, 2)
	if err := f.svc.VoidRun(context.Background(), f.adminID, f.runIDs[0], "first"); err != nil {
		t.Fatalf("void: %v", err)
	}
	problem(t, f.svc.VoidRun(context.Background(), f.adminID, f.runIDs[0], "again"), 409, "already_voided")
}

func TestVoidUnscoredRunIsRefused(t *testing.T) {
	f := setup(t, 1)
	f.exec(t, `UPDATE qualification_runs SET status = 'running', score = NULL WHERE id = $1`, f.runIDs[0])
	problem(t, f.svc.VoidRun(context.Background(), f.adminID, f.runIDs[0], "x"), 409, "run_not_scored")
}

func TestVoidTheOnlyRunLeavesTheAgentUnrated(t *testing.T) {
	f := setup(t, 1)
	if err := f.svc.VoidRun(context.Background(), f.adminID, f.runIDs[0], "fabricated"); err != nil {
		t.Fatalf("void: %v", err)
	}
	// No run, no earlier version: there is no number to show, so the row goes.
	if got := f.rating(t); got.Exists {
		t.Fatalf("rating still stored after its only run was voided: %+v", got)
	}
	rows, err := arena.NewService(f.d.AppPool, 3).Leaderboard(context.Background(), "go", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("leaderboard = %+v, want empty", rows)
	}
}

func TestVoidRunOnAReplacedVersionLeavesTheRatingAlone(t *testing.T) {
	f := setup(t, 2)
	// The owner switched configuration: the stored rating now belongs to v2, and
	// the voided run's evidence is no longer part of it.
	newVersion := idgen.New("ver")
	f.exec(t, `INSERT INTO agent_versions (id, agent_id, number, model, harness, config_digest)
		VALUES ($1, $2, 2, 'claude-sonnet-5-5', 'claude-code 2.1', $1)`, newVersion, f.agentID)
	f.exec(t, `UPDATE agents SET current_version_id = $2 WHERE id = $1`, f.agentID, newVersion)
	f.exec(t, `UPDATE skill_ratings SET version_id = $2, runs = 0, sum_targets = 0, prior_rating = rating
		WHERE agent_id = $1`, f.agentID, newVersion)
	before := f.rating(t)

	if err := f.svc.VoidRun(context.Background(), f.adminID, f.runIDs[1], "late report"); err != nil {
		t.Fatalf("void: %v", err)
	}
	if after := f.rating(t); after != before {
		t.Fatalf("rating changed: %+v -> %+v; a run on a replaced version is no longer part of it", before, after)
	}
	if st := f.runStatus(t, f.runIDs[1]); st != "voided" {
		t.Errorf("the run must still be marked voided, got %q", st)
	}
}

func TestRetireTaskKeepsEveryRatingEarnedOnIt(t *testing.T) {
	f := setup(t, 2)
	before := f.rating(t)
	if err := f.svc.RetireTask(context.Background(), f.adminID, f.taskSlugs[0], "leaked on a forum"); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if after := f.rating(t); after != before {
		t.Fatalf("retiring a task changed a rating: %+v -> %+v", before, after)
	}
	if !f.audited(t, "admin.task_retired", f.taskSlugs[0]) {
		t.Error("retiring must leave one audit event carrying its reason")
	}
	problem(t, f.svc.RetireTask(context.Background(), f.adminID, "no-such-task", "x"), 404, "not_found")
}

func TestTaskStatsAreAdminOnlyData(t *testing.T) {
	f := setup(t, 1)
	stats, err := f.svc.TaskStats(context.Background(), "go")
	if err != nil {
		t.Fatalf("task stats: %v", err)
	}
	if len(stats) != 3 {
		t.Fatalf("stats = %d entries, want 3", len(stats))
	}
}

func TestBanAgentHidesItAndUnbanRestoresIt(t *testing.T) {
	f := setup(t, 1)
	svc := arena.NewService(f.d.AppPool, 3)

	if err := f.svc.BanAgent(context.Background(), f.adminID, f.agentID, "a human was doing the work"); err != nil {
		t.Fatalf("ban: %v", err)
	}
	rows, err := svc.Leaderboard(context.Background(), "go", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("a banned agent is still on the leaderboard: %+v", rows)
	}
	if !f.audited(t, "admin.agent_banned", f.agentID) {
		t.Error("banning must leave one audit event carrying its reason")
	}

	if err := f.svc.UnbanAgent(context.Background(), f.adminID, f.agentID, "appeal upheld"); err != nil {
		t.Fatalf("unban: %v", err)
	}
	if rows, err = svc.Leaderboard(context.Background(), "go", 10); err != nil || len(rows) != 1 {
		t.Fatalf("after unban: rows = %+v, err = %v; want the agent back", rows, err)
	}
	problem(t, f.svc.BanAgent(context.Background(), f.adminID, "agent_nope", "x"), 404, "not_found")
}
