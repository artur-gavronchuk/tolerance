package qualifications_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
	"tolerance/internal/qualifications"
	"tolerance/internal/skillrating"
	"tolerance/internal/skills"
)

// fx wires both halves of the platform the way cmd/api does: the API side (Start, Get, Claim, heartbeat and
// version changes) on arena_app, and the worker side (RunProof, the finish hook, ExpireStale, the stalled
// sweep) on arena_worker, so the worker's grants are exercised by every test.
type fx struct {
	d       *dbtest.DB
	agents  *agents.Service
	proofs  *proofs.Service // API side
	quals   *qualifications.Service
	wproofs *proofs.Service // worker side
	wquals  *qualifications.Service
	worker  *proofs.Worker
	fake    *sandbox.Fake
	hidden  map[string][]string // skill task slug -> hidden test names
	tasks   map[string]skills.Task
	userID  string
	agentID string
}

// noteDiff applies to any task repository: it adds a file no test reads.
// git apply refuses a patch without changes, so tests cannot send "--- a/x".
const noteDiff = "diff --git a/NOTES.md b/NOTES.md\nnew file mode 100644\n--- /dev/null\n+++ b/NOTES.md\n@@ -0,0 +1 @@\n+agent notes\n"

func passing(names []string) []sandbox.TestResult {
	out := make([]sandbox.TestResult, len(names))
	for i, n := range names {
		out[i] = sandbox.TestResult{Name: n, Passed: true}
	}
	return out
}

func logger() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stderr, nil)) }

// fixturePool is how many tasks backend/fixtures/skills holds per skill.
const fixturePool = 3

func setup(t *testing.T) *fx {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	sk, tasks, err := skills.LoadCatalog(filepath.Join("..", "..", "fixtures", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if err := skills.SyncCatalog(ctx, d.AdminPool, sk, tasks); err != nil {
		t.Fatal(err)
	}
	hidden, err := skills.HiddenNamesByTask(sk, tasks)
	if err != nil {
		t.Fatal(err)
	}
	byslug := map[string]skills.Task{}
	for _, tk := range tasks {
		byslug[tk.Slug] = tk
	}
	ptasks, err := proofs.LoadCatalog(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatal(err)
	}
	if err := proofs.SyncCatalog(ctx, d.AdminPool, ptasks); err != nil {
		t.Fatal(err)
	}

	ps := proofs.NewService(d.AppPool)
	qs := qualifications.NewService(d.AppPool, ps)
	// This repository's practice catalog holds fixturePool tasks per skill; the
	// production floor (skills.MinPool) is meant for the larger private rating
	// catalog and would freeze every skill here. Tests that need the freeze raise
	// the floor themselves.
	qs.SetMinPool(fixturePool)
	as := agents.NewService(d.AppPool, ps)
	as.SetVersionListener(qs)

	wps := proofs.NewService(d.WorkerPool)
	wqs := qualifications.NewService(d.WorkerPool, wps)
	wqs.SetMinPool(fixturePool)
	fake := &sandbox.Fake{}
	w := proofs.NewWorker(d.WorkerPool, fake, t.TempDir(), logger())
	w.SetFinishListener(wqs)

	us := identity.NewService(d.AppPool, nil)
	u, _, err := us.SignIn(ctx, identity.Identity{Provider: "dev", Subject: "o@example.com", Email: "o@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	a, err := as.Create(ctx, u.ID, agents.CreateInput{Name: "fixer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := as.CreateKey(ctx, u.ID, "k"); err != nil {
		t.Fatal(err)
	}
	if err := as.Heartbeat(ctx, a.ID, "0.2", "h"); err != nil {
		t.Fatal(err)
	}
	return &fx{d: d, agents: as, proofs: ps, quals: qs, wproofs: wps, wquals: wqs, worker: w, fake: fake, hidden: hidden, tasks: byslug, userID: u.ID, agentID: a.ID}
}

func (f *fx) admin(t *testing.T, sql string, args ...any) {
	t.Helper()
	err := f.d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f *fx) operational(t *testing.T) {
	t.Helper()
	f.admin(t, `INSERT INTO proofs (id, agent_id, kind, task_slug, status, finished_at) VALUES ('proof_seed', $1, 'proof', 'go-fix-retry', 'passed', now())`, f.agentID)
}

func (f *fx) version(t *testing.T, digest string) agents.Version {
	t.Helper()
	v, _, err := f.agents.EnsureVersion(context.Background(), f.agentID, agents.VersionInput{Model: "m", Harness: "h", ConfigDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f *fx) start(t *testing.T, skill string) qualifications.Run {
	t.Helper()
	run, err := f.quals.Start(context.Background(), f.userID, skill)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func (f *fx) get(t *testing.T, id string) qualifications.Run {
	t.Helper()
	run, err := f.quals.Get(context.Background(), f.userID, id)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func problem(t *testing.T, err error, status int, code string) {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) || p.Status != status || p.Code != code {
		t.Fatalf("expected %d %s, got %v", status, code, err)
	}
}

// age moves the run's finished proofs two minutes into the past.
func (f *fx) age(t *testing.T, runID string) {
	t.Helper()
	f.admin(t, `UPDATE proofs SET finished_at = finished_at - interval '2 minutes' WHERE qualification_run_id = $1 AND finished_at IS NOT NULL`, runID)
}

// claimAndSubmit plays the connector for the run's current task: claim it and post a diff.
func (f *fx) claimAndSubmit(t *testing.T) (*proofs.Proof, *proofs.Task) {
	t.Helper()
	ctx := context.Background()
	p, task, err := f.proofs.Claim(ctx, f.agentID)
	if err != nil || p == nil {
		t.Fatalf("claim: %v %+v", err, p)
	}
	if p.Kind != proofs.KindQualification || task.TaskMD == "" {
		t.Fatalf("expected a qualification task, got %+v", p)
	}
	if err := f.proofs.SubmitResult(ctx, f.agentID, p.ID, proofs.ResultInput{Diff: noteDiff}); err != nil {
		t.Fatal(err)
	}
	return p, task
}

// drive plays the connector and the sandbox for the run's current task: the given number of its hidden
// tests pass, and so do two tests outside the hidden set (a visible one and one the agent might have
// added), which must never add to the score. It returns the slug of the task it drove.
func (f *fx) drive(t *testing.T, pass float64) string {
	t.Helper()
	p, task := f.claimAndSubmit(t)
	names := f.hidden[task.Slug]
	n := int(math.Round(pass * float64(len(names))))
	tests := []sandbox.TestResult{{Name: "visible_passes", Passed: true}, {Name: "test_agent_added.py::test_extra", Passed: true}}
	for i, name := range names {
		tests = append(tests, sandbox.TestResult{Name: name, Passed: i < n})
	}
	exit := 0
	if n < len(names) {
		exit = 1
	}
	f.fake.Result = sandbox.Result{ExitCode: exit, Tests: tests}
	f.fake.Err = nil
	if err := f.worker.RunProof(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	return task.Slug
}

func TestStart_Guards(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.quals.Start(ctx, f.userID, "go")
	problem(t, err, 409, "agent_not_operational")
	f.operational(t)
	_, err = f.quals.Start(ctx, f.userID, "go")
	problem(t, err, 409, "no_version")
	f.version(t, "d1")
	_, err = f.quals.Start(ctx, f.userID, "rust")
	problem(t, err, 404, "unknown_skill")
	run, err := f.quals.Start(ctx, f.userID, "go")
	if err != nil || run.Status != "running" || len(run.TaskSlugs) != 3 || len(run.Tasks) != 1 {
		t.Fatalf("start: %v %+v", err, run)
	}
	_, err = f.quals.Start(ctx, f.userID, "python")
	problem(t, err, 409, "qualification_in_progress")
}

func TestRun_ThreeTasksScoreAndRating(t *testing.T) {
	f := setup(t)
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "go")
	shares := []float64{1, 0.5, 0}
	var weighted, weights float64
	for i, share := range shares {
		slug := f.drive(t, share)
		if slug != run.TaskSlugs[i] {
			t.Fatalf("task %d: driven %s, run says %s", i+1, slug, run.TaskSlugs[i])
		}
		n := float64(len(f.hidden[slug]))
		d := float64(f.tasks[slug].Difficulty)
		weighted += d * math.Round(share*n) / n
		weights += d
	}
	// The score is stored with four decimals (numeric(5,4)); the rating is computed from the stored value.
	want := math.Round(weighted/weights*1e4) / 1e4
	got := f.get(t, run.ID)
	if got.Status != "scored" || got.Score == nil || len(got.Tasks) != 3 {
		t.Fatalf("%+v", got)
	}
	if math.Abs(*got.Score-want) > 1e-9 {
		t.Fatalf("score %v, want %v", *got.Score, want)
	}
	next := skillrating.Apply(skillrating.State{}, want)
	rs, err := f.quals.RatingsFor(context.Background(), f.agentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].SkillSlug != "go" || rs[0].Runs != 1 || rs[0].Uncertainty != 350 || !rs[0].OnCurrentVersion || rs[0].Rating != next.Rating {
		t.Fatalf("ratings: %+v, want rating %d", rs, next.Rating)
	}
	if got.RatingAfter == nil || *got.RatingAfter != next.Rating || got.UncertaintyAfter == nil || *got.UncertaintyAfter != 350 || got.RatingBefore != nil {
		t.Fatalf("run rating fields: %+v", got)
	}
	for _, tk := range got.Tasks {
		if tk.SandboxResult == nil {
			continue
		}
		if tk.SandboxResult.Output != "" {
			t.Fatalf("sandbox output must not be exposed: %+v", tk.SandboxResult)
		}
		for i, tr := range tk.SandboxResult.Tests {
			if tr.Name != fmt.Sprintf("hidden-%d", i+1) {
				t.Fatalf("hidden test names must be masked at every index: %+v", tk.SandboxResult.Tests)
			}
		}
		for _, name := range f.hidden[*tk.SkillTaskSlug] {
			for _, tr := range tk.SandboxResult.Tests {
				if tr.Name == name {
					t.Fatalf("hidden name %q leaked", name)
				}
			}
		}
	}
}

func TestRun_InfraErrorRequeuesOnceThenExcludes(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "python")
	// task 1: infra error twice -> excluded
	for i := 0; i < 2; i++ {
		p, _ := f.claimAndSubmit(t)
		f.fake.Err = errors.New("no docker")
		if err := f.worker.RunProof(ctx, p.ID); err == nil {
			t.Fatal("a sandbox failure must surface as a job error")
		}
		if err := f.worker.MarkInfraError(ctx, p.ID, "no docker"); err != nil {
			t.Fatal(err)
		}
	}
	f.drive(t, 1)
	f.drive(t, 1)
	got := f.get(t, run.ID)
	if got.Status != "scored" || got.Score == nil || *got.Score != 1 || len(got.Tasks) != 4 {
		t.Fatalf("two clean tasks must average to 1.0 with the infra-errored one excluded: %+v", got)
	}
}

func TestRun_AbortWhenTaskExpiresAndDailyLimit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "go")
	f.admin(t, `UPDATE proofs SET created_at = now() - interval '6 minutes' WHERE qualification_run_id = $1`, run.ID)
	ids, err := f.wproofs.ExpireStale(ctx)
	if err != nil || len(ids) != 1 {
		t.Fatalf("expire: %v %v", ids, err)
	}
	for _, id := range ids {
		if err := f.wquals.OnProofFinished(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.get(t, run.ID); got.Status != "aborted" {
		t.Fatalf("expired task must abort the run: %+v", got)
	}
	rs, err := f.quals.RatingsFor(ctx, f.agentID)
	if err != nil || len(rs) != 0 {
		t.Fatalf("aborted run must not create a rating: %v %v", rs, err)
	}
	// The aborted run above counts: two more make three today.
	for i := 0; i < 2; i++ {
		r := f.start(t, "go")
		if err := f.quals.Abort(ctx, r.ID, "test"); err != nil {
			t.Fatal(err)
		}
	}
	_, err = f.quals.Start(ctx, f.userID, "go")
	problem(t, err, 429, "daily_limit")
}

func TestStart_NeedsOnlineConnector(t *testing.T) {
	f := setup(t)
	f.operational(t)
	f.version(t, "d1")
	f.admin(t, `UPDATE agent_presence SET last_seen_at = now() - interval '3 minutes' WHERE agent_id = $1`, f.agentID)
	_, err := f.quals.Start(context.Background(), f.userID, "go")
	problem(t, err, 409, "agent_offline")
}

func TestScore_CountsOnlyHiddenTestsByName(t *testing.T) {
	f := setup(t)
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "python")
	for i := 0; i < 3; i++ {
		f.drive(t, 0) // every hidden test fails; the two tests outside the hidden set pass
	}
	got := f.get(t, run.ID)
	if got.Status != "scored" || got.Score == nil || *got.Score != 0 || got.RatingAfter == nil || *got.RatingAfter != 1000 {
		t.Fatalf("tests outside the hidden set must not score: %+v", got)
	}
}

func TestRun_HoldsTheSlotAndSweepsStalled(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "go")

	// Task 1 finishes on a worker without the hook, as if the API stopped
	// between the verdict and the advance.
	deaf := proofs.NewWorker(f.d.WorkerPool, f.fake, t.TempDir(), logger())
	p1, task := f.claimAndSubmit(t)
	f.fake.Result = sandbox.Result{Tests: passing(f.hidden[task.Slug])}
	f.fake.Err = nil
	if err := deaf.RunProof(ctx, p1.ID); err != nil {
		t.Fatal(err)
	}

	// No proof is open, yet the run keeps the slot.
	_, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	problem(t, err, 409, "qualification_in_progress")
	last, err := f.proofs.Latest(ctx, f.agentID)
	if err != nil || last == nil || last.ID != "proof_seed" {
		t.Fatalf("last_proof must stay the basic proof: %v %+v", err, last)
	}

	if n, err := f.wquals.SweepStalled(ctx); err != nil || n != 0 {
		t.Fatalf("a proof finished seconds ago is not stalled yet: %d %v", n, err)
	}
	f.age(t, run.ID)
	if n, err := f.wquals.SweepStalled(ctx); err != nil || n != 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	if err := f.wquals.OnProofFinished(ctx, p1.ID); err != nil { // the late hook is a no-op
		t.Fatal(err)
	}
	got := f.get(t, run.ID)
	if len(got.Tasks) != 2 || *got.Tasks[1].Position != 2 {
		t.Fatalf("exactly one next task: %+v", got.Tasks)
	}

	// Task 2's result is too big: FailOversized closes it outside the worker.
	p2, _, err := f.proofs.Claim(ctx, f.agentID)
	if err != nil || p2 == nil {
		t.Fatalf("claim: %v %v", err, p2)
	}
	err = f.proofs.SubmitResult(ctx, f.agentID, p2.ID, proofs.ResultInput{Diff: strings.Repeat("+x\n", 100_000)})
	problem(t, err, 413, "diff_too_large")
	_, err = f.proofs.Retry(ctx, f.userID, p2.ID)
	problem(t, err, 409, "state_conflict")
	f.age(t, run.ID)
	if n, err := f.wquals.SweepStalled(ctx); err != nil || n != 1 {
		t.Fatalf("sweep after oversized: %d %v", n, err)
	}
	got = f.get(t, run.ID)
	if len(got.Tasks) != 3 || *got.Tasks[2].Position != 3 {
		t.Fatalf("an oversized task 2 must still move the run on: %+v", got.Tasks)
	}
}

// The hook and the sweep may both fire for the same proof, on several
// replicas at once: the run must still get exactly one next task.
func TestOnProofFinished_ConcurrentAndRepeatedIsIdempotent(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "go")
	deaf := proofs.NewWorker(f.d.WorkerPool, f.fake, t.TempDir(), logger())
	p1, task := f.claimAndSubmit(t)
	f.fake.Result = sandbox.Result{Tests: passing(f.hidden[task.Slug])}
	if err := deaf.RunProof(ctx, p1.ID); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		go func() { errs <- f.wquals.OnProofFinished(ctx, p1.ID) }()
	}
	for i := 0; i < 6; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if got := f.get(t, run.ID); len(got.Tasks) != 2 {
		t.Fatalf("exactly one next task expected: %+v", got.Tasks)
	}
	// A non-qualification proof is ignored.
	if err := f.wquals.OnProofFinished(ctx, "proof_seed"); err != nil {
		t.Fatal(err)
	}
}

func TestNewVersion_ResetsConfidenceKeepsPrior(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "go")
	f.drive(t, 1)
	f.drive(t, 1)
	f.drive(t, 1)
	if got := f.get(t, run.ID); got.RatingAfter == nil || *got.RatingAfter != 2400 {
		t.Fatalf("perfect run must rate 2400, got %+v", got)
	}
	f.version(t, "d2")
	rs, err := f.quals.RatingsFor(ctx, f.agentID)
	if err != nil || len(rs) != 1 {
		t.Fatalf("%v %v", rs, err)
	}
	if rs[0].Rating != 2400 || rs[0].Uncertainty != 350 || rs[0].Runs != 0 || rs[0].OnCurrentVersion || rs[0].PriorRating == nil {
		t.Fatalf("after version change: %+v", rs[0])
	}
	if rs[0].Verified {
		t.Fatalf("2400-350 = 2050 is verified by access, but the rating is not on the current version and must not count as verified")
	}

	// A version change during a running run aborts it: the rating stays as it was, the open task expires.
	run2 := f.start(t, "go")
	f.version(t, "d3")
	got := f.get(t, run2.ID)
	if got.Status != "aborted" || got.Score != nil || got.RatingAfter != nil || got.FinishedAt == nil {
		t.Fatalf("a version change must abort the running run: %+v", got)
	}
	if len(got.Tasks) != 1 || got.Tasks[0].Status != proofs.StatusExpired || got.Tasks[0].FailureReason != "run_aborted: version_changed" {
		t.Fatalf("the open qualification proof must expire: %+v", got.Tasks)
	}
	after, err := f.quals.RatingsFor(ctx, f.agentID)
	if err != nil || len(after) != 1 || after[0].Rating != 2400 || after[0].Runs != 0 || after[0].Uncertainty != 350 {
		t.Fatalf("rating must be untouched by the aborted run: %v %+v", err, after)
	}
	// The slot is free again.
	if _, err := f.quals.Start(ctx, f.userID, "go"); err != nil {
		t.Fatalf("a new run must be possible after the abort: %v", err)
	}
}

// infraOut makes the run's current task end in infra_error.
func (f *fx) infraOut(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	p, _ := f.claimAndSubmit(t)
	f.fake.Err = errors.New("no docker")
	if err := f.worker.RunProof(ctx, p.ID); err == nil {
		t.Fatal("a sandbox failure must surface as a job error")
	}
	if err := f.worker.MarkInfraError(ctx, p.ID, "no docker"); err != nil {
		t.Fatal(err)
	}
}

func TestRun_AllTasksExcludedAbortsWithoutRating(t *testing.T) {
	f := setup(t)
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "go")
	for i := 0; i < 6; i++ { // every task: infra error, one re-run, infra error again
		f.infraOut(t)
	}
	got := f.get(t, run.ID)
	if got.Status != "aborted" || got.Score != nil || got.RatingAfter != nil {
		t.Fatalf("a run with nothing to score must abort: %+v", got)
	}
	rs, err := f.quals.RatingsFor(context.Background(), f.agentID)
	if err != nil || len(rs) != 0 {
		t.Fatalf("no rating expected: %v %v", rs, err)
	}
}

// A task that expires after the connector claimed it scores 0 and the run goes on; only a task nobody
// picked up (not_claimed) aborts the run.
func TestRun_ClaimedTaskExpiryScoresZeroAndRunContinues(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "go")
	p, _, err := f.proofs.Claim(ctx, f.agentID)
	if err != nil || p == nil {
		t.Fatalf("claim: %v %v", err, p)
	}
	f.admin(t, `UPDATE proofs SET claimed_at = now() - interval '2 hours' WHERE id = $1`, p.ID)
	ids, err := f.wproofs.ExpireStale(ctx)
	if err != nil || len(ids) != 1 || ids[0] != p.ID {
		t.Fatalf("expire: %v %v", ids, err)
	}
	if err := f.wquals.OnProofFinished(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, run.ID); got.Status != "running" || len(got.Tasks) != 2 || got.Tasks[0].FailureReason != "agent_timeout" {
		t.Fatalf("the run must go on to task 2: %+v", got)
	}
	f.drive(t, 1)
	f.drive(t, 1)
	got := f.get(t, run.ID)
	if got.Status != "scored" || got.Score == nil {
		t.Fatalf("%+v", got)
	}
	var weighted, weights float64
	for i, slug := range run.TaskSlugs {
		d := float64(f.tasks[slug].Difficulty)
		weights += d
		if i > 0 {
			weighted += d
		}
	}
	want := math.Round(weighted/weights*1e4) / 1e4
	if math.Abs(*got.Score-want) > 1e-9 {
		t.Fatalf("score %v, want %v", *got.Score, want)
	}
}

// A run that has been running far longer than its tasks could take is aborted by the sweep even when its
// advance cannot succeed, and frees the slot.
func TestSweepStalled_AbortsOverdueRun(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "go")
	p, _, err := f.proofs.Claim(ctx, f.agentID)
	if err != nil || p == nil {
		t.Fatalf("claim: %v %v", err, p)
	}
	f.admin(t, `UPDATE qualification_runs SET created_at = now() - interval '1 day' WHERE id = $1`, run.ID)
	// Break the advance for good: its next task slug no longer exists.
	f.admin(t, `UPDATE proofs SET status = 'failed', finished_at = now() - interval '2 minutes' WHERE id = $1`, p.ID)
	f.admin(t, `UPDATE qualification_runs SET task_slugs = task_slugs[1:1] WHERE id = $1`, run.ID)
	if n, err := f.wquals.SweepStalled(ctx); err != nil || n != 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	got := f.get(t, run.ID)
	if got.Status != "aborted" || got.RatingAfter != nil {
		t.Fatalf("%+v", got)
	}
	if _, err := f.quals.Start(ctx, f.userID, "go"); err != nil {
		t.Fatalf("the slot must be free: %v", err)
	}
}

func TestRun_SlotRefusesOtherProofKinds(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	f.start(t, "go")
	_, err := f.proofs.CreateWithRepo(ctx, f.userID, "tanks-bot", []byte("x"))
	problem(t, err, 409, "qualification_in_progress")
	_, err = f.proofs.Retry(ctx, f.userID, "proof_seed")
	problem(t, err, 409, "qualification_in_progress")
}

// exposures reads how many distinct agents the task has been handed to.
func (f *fx) exposures(t *testing.T, slug string) int {
	t.Helper()
	var n int
	err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT exposures FROM skill_tasks WHERE slug = $1`, slug).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestStart_FrozenSkillRefuses(t *testing.T) {
	f := setup(t)
	f.operational(t)
	f.version(t, "d1")
	// One task more than the catalog holds: the pool is below the floor.
	f.quals.SetMinPool(fixturePool + 1)
	_, err := f.quals.Start(context.Background(), f.userID, "go")
	problem(t, err, 409, "skill_frozen")
}

func TestRun_InfraRequeueDoesNotWidenExposure(t *testing.T) {
	f := setup(t)
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "go")
	first := run.TaskSlugs[0]
	if got := f.exposures(t, first); got != 1 {
		t.Fatalf("exposures after handing the task out once = %d, want 1", got)
	}
	// The platform's own re-run hands the same task to the same agent again.
	f.infraOut(t)
	if got := f.exposures(t, first); got != 1 {
		t.Fatalf("exposures after a requeue to the same agent = %d, want 1: a retry by the platform does not widen the leak", got)
	}
}

// advanced asserts that the run has n finished proofs. The worker logs and
// swallows a failing finish hook (proofs.Worker.notify), so without this a
// stalled advance shows up only as a mysteriously unscored run several steps
// later; here it fails at the step that stalled.
func (f *fx) advanced(t *testing.T, runID string, n int) {
	t.Helper()
	var got int
	err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM proofs WHERE qualification_run_id = $1 AND finished_at IS NOT NULL`, runID).Scan(&got)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != n {
		t.Fatalf("run %s has %d finished proofs, want %d: the advance stalled", runID, got, n)
	}
}

func TestRun_FinishesOnTasksRetiredMidRun(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "go")
	// Every task of the skill leaves the pool while the run is in flight.
	f.admin(t, `UPDATE skill_tasks SET retired_at = now(), retired_reason = 'test' WHERE skill_slug = 'go'`)
	for i := 1; i <= 3; i++ {
		f.drive(t, 1)
		f.advanced(t, run.ID, i)
	}
	got := f.get(t, run.ID)
	if got.Status != "scored" || got.Score == nil || *got.Score != 1 {
		t.Fatalf("a run already in flight must finish on tasks that retired under it: %+v", got)
	}
	// But no new run may start on the emptied pool.
	_, err := f.quals.Start(ctx, f.userID, "go")
	problem(t, err, 409, "skill_frozen")
}

// taskObserved reads the accumulated observed difficulty of one task.
func (f *fx) taskObserved(t *testing.T, slug string) (runs int, sum float64) {
	t.Helper()
	err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT runs, sum_score::float8 FROM skill_tasks WHERE slug = $1`, slug).Scan(&runs, &sum)
	})
	if err != nil {
		t.Fatal(err)
	}
	return runs, sum
}

func TestScore_AccumulatesObservedDifficultyPerTask(t *testing.T) {
	f := setup(t)
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "go")
	// Three tasks at 1.0, 0.5 and 0.0 of their hidden tests.
	f.drive(t, 1)
	f.drive(t, 0.5)
	f.drive(t, 0)

	got := f.get(t, run.ID)
	if got.Status != "scored" {
		t.Fatalf("run status = %q, want scored", got.Status)
	}
	var total float64
	for i, slug := range run.TaskSlugs {
		runs, sum := f.taskObserved(t, slug)
		if runs != 1 {
			t.Fatalf("task %d (%s): runs = %d, want 1", i+1, slug, runs)
		}
		total += sum
	}
	// 1.0 + 0.5 + 0.0, with each task's own hidden-test count rounding its own
	// share. The point of the accumulation is that observed difficulty can later
	// be compared against the difficulty someone typed by hand.
	if total < 1.4 || total > 1.6 {
		t.Fatalf("sum of observed scores = %v, want about 1.5", total)
	}
}

func TestScore_InfraErroredTaskAddsNoObservation(t *testing.T) {
	f := setup(t)
	f.operational(t)
	f.version(t, "d1")
	run := f.start(t, "python")
	first := run.TaskSlugs[0]
	// Both attempts at task 1 die on the platform, so it is excluded from the run.
	f.infraOut(t)
	f.infraOut(t)
	f.drive(t, 1)
	f.drive(t, 1)

	if runs, sum := f.taskObserved(t, first); runs != 0 || sum != 0 {
		t.Fatalf("an infra-errored task = %d runs, %v sum; want 0, 0: a platform failure says nothing about difficulty", runs, sum)
	}
}

func TestStart_BannedAgentCannotEarnARating(t *testing.T) {
	f := setup(t)
	f.operational(t)
	f.version(t, "d1")
	f.admin(t, `UPDATE agents SET banned_at = now(), banned_reason = 'a human was doing the work' WHERE id = $1`, f.agentID)
	// A ban has to stop the agent competing, not just hide it: otherwise it keeps
	// growing a rating that reappears in full the moment the ban is lifted.
	_, err := f.quals.Start(context.Background(), f.userID, "go")
	problem(t, err, 403, "agent_banned")
}
