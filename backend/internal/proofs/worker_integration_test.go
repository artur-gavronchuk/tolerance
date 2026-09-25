package proofs_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
)

const goodDiff = `--- a/retry.go
+++ b/retry.go
@@ -19,7 +19,14 @@ func Backoff(attempt int) time.Duration {
 	if attempt < 1 {
 		return 0
 	}
-	return Base * time.Duration(attempt)
+	if attempt > 20 {
+		return Max
+	}
+	d := Base << uint(attempt-1)
+	if d > Max {
+		return Max
+	}
+	return d
 }

 // Do calls fn until it succeeds or maxAttempts is used up.
`

// hiddenNames are the test functions in fixtures/proofs/go-fix-retry/_hidden.
var hiddenNames = []string{"TestHidden_BackoffSequence", "TestHidden_BackoffCapsAtMax", "TestHidden_BackoffZeroAndNegative",
	"TestHidden_DoStopsAtMaxAttempts", "TestHidden_DoReturnsContextErrorWhileWaiting"}

// passing returns a test list with the given extra results plus every hidden
// test passing.
func passing(extra ...sandbox.TestResult) []sandbox.TestResult {
	out := append([]sandbox.TestResult{}, extra...)
	for _, n := range hiddenNames {
		out = append(out, sandbox.TestResult{Name: n, Passed: true})
	}
	return out
}

func submitted(t *testing.T, f fixture, diff string) string {
	t.Helper()
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	if _, err := f.proofs.Create(ctx, f.userID, "go-fix-retry"); err != nil {
		t.Fatal(err)
	}
	p, _, _ := f.proofs.Claim(ctx, f.agent)
	if err := f.proofs.SubmitResult(ctx, f.agent, p.ID, proofs.ResultInput{Diff: diff}); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func TestRunProof_PassedFailedAndInfraError(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()

	t.Run("all tests pass", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing(sandbox.TestResult{Name: "TestA", Passed: true})}}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		id := submitted(t, f, goodDiff)
		if err := w.RunProof(ctx, id); err != nil {
			t.Fatal(err)
		}
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusPassed || p.SandboxResult == nil || p.FinishedAt == nil {
			t.Fatalf("%+v", p)
		}
		if len(fake.Calls) != 1 || fake.Calls[0].Image != "arena-proof-go:1" {
			t.Fatalf("runner not called with the task image: %+v", fake.Calls)
		}
		// hidden tests were copied over the applied diff
		if _, err := os.Stat(filepath.Join(fake.Calls[0].WorkDir, "retry_hidden_test.go")); err == nil {
			t.Fatalf("work dir must be removed after the run")
		}
		facts, _ := f.proofs.ProofFacts(ctx, f.agent)
		if !facts.HasPassed {
			t.Fatalf("facts: %+v", facts)
		}
	})

	t.Run("a hidden test fails", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 1, Tests: []sandbox.TestResult{{Name: "TestA", Passed: true}, {Name: "TestHidden_X", Passed: false}}}}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		id := submitted(t, f, goodDiff)
		_ = w.RunProof(ctx, id)
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusFailed || p.FailureReason != "tests_failed" {
			t.Fatalf("%+v", p)
		}
	})

	t.Run("diff does not apply", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		id := submitted(t, f, "--- a/nope.go\n+++ b/nope.go\n@@ -1 +1 @@\n-x\n+y\n")
		_ = w.RunProof(ctx, id)
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusFailed || p.FailureReason != "diff_not_applicable" || len(fake.Calls) != 0 {
			t.Fatalf("%+v calls=%d", p, len(fake.Calls))
		}
	})

	t.Run("a diff escaping the work dir is refused", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing()}}
		workDir := t.TempDir()
		w := proofs.NewWorker(f.d.AppPool, fake, workDir, log)
		escapes := map[string]string{
			"dotdot":  "diff --git a/../escape.txt b/../escape.txt\nnew file mode 100644\n--- /dev/null\n+++ b/../escape.txt\n@@ -0,0 +1 @@\n+pwned\n",
			"plain":   "--- /dev/null\n+++ b/../../escape.txt\n@@ -0,0 +1 @@\n+pwned\n",
			"git":     "diff --git a/.git/config b/.git/config\nnew file mode 100644\n--- /dev/null\n+++ b/.git/config\n@@ -0,0 +1 @@\n+pwned\n",
			"symlink": "diff --git a/link b/link\nnew file mode 120000\n--- /dev/null\n+++ b/link\n@@ -0,0 +1 @@\n+" + workDir + "\n\\ No newline at end of file\n",
		}
		for name, diff := range escapes {
			id := submitted(t, f, diff)
			if err := w.RunProof(ctx, id); err != nil {
				t.Fatal(err)
			}
			p, _ := f.proofs.Get(ctx, f.userID, id)
			if p.Status != proofs.StatusFailed || p.FailureReason != "diff_not_applicable" {
				t.Fatalf("%s: %+v", name, p)
			}
		}
		if len(fake.Calls) != 0 {
			t.Fatalf("sandbox must not run: %d calls", len(fake.Calls))
		}
		for _, p := range []string{filepath.Join(workDir, "escape.txt"), filepath.Join(filepath.Dir(workDir), "escape.txt")} {
			if _, err := os.Stat(p); err == nil {
				t.Fatalf("patch wrote outside the work dir: %s", p)
			}
		}
	})

	t.Run("hidden tests that did not run fail the proof", func(t *testing.T) {
		f := setup(t)
		// Everything that ran passed, but only the visible tests ran.
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: []sandbox.TestResult{
			{Name: "TestDo_SucceedsFirstTry", Passed: true}, {Name: "TestBackoff_Doubles", Passed: true}}}}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		id := submitted(t, f, goodDiff)
		if err := w.RunProof(ctx, id); err != nil {
			t.Fatal(err)
		}
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusFailed || p.FailureReason != "hidden_test_missing_or_failed" {
			t.Fatalf("%+v", p)
		}
		// One hidden test missing is enough.
		f2 := setup(t)
		fake2 := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing()[1:]}}
		w2 := proofs.NewWorker(f2.d.AppPool, fake2, t.TempDir(), log)
		id2 := submitted(t, f2, goodDiff)
		_ = w2.RunProof(ctx, id2)
		p, _ = f2.proofs.Get(ctx, f2.userID, id2)
		if p.Status != proofs.StatusFailed || p.FailureReason != "hidden_test_missing_or_failed" {
			t.Fatalf("%+v", p)
		}
	})

	t.Run("a diff touching a test file is refused", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing()}}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		testMain := "diff --git a/main_test.go b/main_test.go\nnew file mode 100644\n--- /dev/null\n+++ b/main_test.go\n@@ -0,0 +1,3 @@\n+package retry\n+\n+// TestMain here\n"
		id := submitted(t, f, goodDiff+testMain)
		if err := w.RunProof(ctx, id); err != nil {
			t.Fatal(err)
		}
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusFailed || p.FailureReason != "test_file_modified" || len(fake.Calls) != 0 {
			t.Fatalf("%+v calls=%d", p, len(fake.Calls))
		}
	})

	t.Run("CRLF diff still applies", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing()}}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		crlf := ""
		for _, line := range splitLines(goodDiff) {
			crlf += line + "\r\n"
		}
		id := submitted(t, f, crlf)
		_ = w.RunProof(ctx, id)
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusPassed {
			t.Fatalf("CRLF diff: %+v", p)
		}
	})

	t.Run("sandbox timeout is a failure", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{TimedOut: true, ExitCode: -1}}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		id := submitted(t, f, goodDiff)
		_ = w.RunProof(ctx, id)
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusFailed || p.FailureReason != "timeout" {
			t.Fatalf("%+v", p)
		}
	})

	t.Run("runner error is infra_error after the job gives up", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Err: errors.New("no docker")}
		w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)
		id := submitted(t, f, goodDiff)
		if err := w.RunProof(ctx, id); err == nil {
			t.Fatalf("runner error must propagate so the job retries")
		}
		p, _ := f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusRunningSandbox {
			t.Fatalf("before giving up the proof stays running_sandbox: %+v", p)
		}
		if err := w.MarkInfraError(ctx, id, "no docker"); err != nil {
			t.Fatal(err)
		}
		p, _ = f.proofs.Get(ctx, f.userID, id)
		if p.Status != proofs.StatusInfraError || p.FailureReason == "" {
			t.Fatalf("%+v", p)
		}
	})
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func TestExpireStale(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	p, _ := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET created_at = now() - interval '6 minutes' WHERE id = $1`, p.ID)
		return err
	})
	n, err := f.proofs.ExpireStale(ctx)
	if err != nil || n != 1 {
		t.Fatalf("expire: %v %d", err, n)
	}
	got, _ := f.proofs.Get(ctx, f.userID, p.ID)
	if got.Status != proofs.StatusExpired || got.FailureReason != "not_claimed" {
		t.Fatalf("%+v", got)
	}

	p2, _ := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	_, _, _ = f.proofs.Claim(ctx, f.agent)
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET claimed_at = now() - interval '20 minutes' WHERE id = $1`, p2.ID)
		return err
	})
	if n, _ := f.proofs.ExpireStale(ctx); n != 1 {
		t.Fatalf("expected the overdue claimed proof to expire, got %d", n)
	}
	got, _ = f.proofs.Get(ctx, f.userID, p2.ID)
	if got.Status != proofs.StatusExpired || got.FailureReason != "agent_timeout" {
		t.Fatalf("%+v", got)
	}
	_ = time.Second
}

// setJobState is a test helper that forces the run_proof job for proofID
// into state, simulating either a long backlog (queued/leased, however old)
// or a worker having given up (failed).
func setJobState(t *testing.T, f fixture, proofID, state string) {
	t.Helper()
	err := f.d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE jobs SET state = $2 WHERE kind = 'run_proof' AND payload->>'proof_id' = $1`, proofID, state)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			t.Fatalf("no run_proof job found for proof %s", proofID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A proof merely waiting its turn in a long job backlog — its run_proof job
// still queued or leased, no matter how long ago the diff was submitted —
// must never be swept as stuck; only a proof whose job has no active
// (queued or leased) run left is genuinely dead.
func TestExpireStale_StuckSandboxRunIsInfraError(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	id := submitted(t, f, goodDiff)
	old := func() {
		_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE proofs SET diff_submitted_at = now() - interval '2 hours' WHERE id = $1`, id)
			return err
		})
	}

	// Still queued behind a long backlog: not stuck, however old.
	old()
	if n, err := f.proofs.ExpireStale(ctx); err != nil || n != 0 {
		t.Fatalf("a job still queued must not be swept, got n=%d err=%v", n, err)
	}

	// Picked up and being worked (leased): still not stuck.
	setJobState(t, f, id, "leased")
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'running_sandbox' WHERE id = $1`, id)
		return err
	})
	old()
	if n, err := f.proofs.ExpireStale(ctx); err != nil || n != 0 {
		t.Fatalf("a leased (in-progress) job must not be swept, got n=%d err=%v", n, err)
	}

	// The job has exhausted its attempts (or the worker crashed before
	// recording the verdict): genuinely dead, must be swept.
	setJobState(t, f, id, "failed")
	old()
	if n, err := f.proofs.ExpireStale(ctx); err != nil || n != 1 {
		t.Fatalf("sweep: %v %d", err, n)
	}
	p, _ := f.proofs.Get(ctx, f.userID, id)
	if p.Status != proofs.StatusInfraError || p.FailureReason != "stuck" || p.FinishedAt == nil {
		t.Fatalf("%+v", p)
	}
	if _, err := f.proofs.Retry(ctx, f.userID, id); err != nil {
		t.Fatalf("a stuck proof must be retriable: %v", err)
	}
}

// A proof whose run_proof job was never created at all (should not happen
// given SubmitResult enqueues it in the same transaction, but is the same
// "no active job" signal as an exhausted one) is swept the same way once
// past the time floor.
func TestExpireStale_NoJobAtAllIsAlsoStuck(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	id := submitted(t, f, goodDiff)
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM jobs WHERE kind = 'run_proof' AND payload->>'proof_id' = $1`, id)
		return err
	})
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET diff_submitted_at = now() - interval '2 hours' WHERE id = $1`, id)
		return err
	})
	if n, err := f.proofs.ExpireStale(ctx); err != nil || n != 1 {
		t.Fatalf("a proof with no job at all must be swept once past the floor: n=%d err=%v", n, err)
	}
}

// A retried proof keeps its id; resubmitting it must enqueue a new run
// rather than resolve to the first, already finished job.
func TestRetry_ResubmitRunsAgain(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	q := jobs.New(f.d.AppPool)
	fake := &sandbox.Fake{Err: errors.New("no docker")}
	w := proofs.NewWorker(f.d.AppPool, fake, t.TempDir(), log)

	id := submitted(t, f, goodDiff)
	job, err := q.Claim(ctx, "test", []string{"run_proof"}, time.Minute)
	if err != nil || job == nil {
		t.Fatalf("first job: %v %v", job, err)
	}
	if err := w.RunProof(ctx, id); err == nil {
		t.Fatal("runner error must propagate")
	}
	if err := q.Complete(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := w.MarkInfraError(ctx, id, "no docker"); err != nil {
		t.Fatal(err)
	}

	if _, err := f.proofs.Retry(ctx, f.userID, id); err != nil {
		t.Fatal(err)
	}
	p, _, err := f.proofs.Claim(ctx, f.agent)
	if err != nil || p == nil || p.ID != id {
		t.Fatalf("claim after retry: %v %+v", err, p)
	}
	if err := f.proofs.SubmitResult(ctx, f.agent, id, proofs.ResultInput{Diff: goodDiff}); err != nil {
		t.Fatal(err)
	}
	second, err := q.Claim(ctx, "test", []string{"run_proof"}, time.Minute)
	if err != nil || second == nil || second.ID == job.ID {
		t.Fatalf("resubmission must enqueue a fresh job: %+v %v", second, err)
	}

	fake.Err, fake.Result = nil, sandbox.Result{ExitCode: 0, Tests: passing()}
	if err := w.RunProof(ctx, id); err != nil {
		t.Fatal(err)
	}
	got, _ := f.proofs.Get(ctx, f.userID, id)
	if got.Status != proofs.StatusPassed || len(fake.Calls) != 2 {
		t.Fatalf("second run: %+v calls=%d", got, len(fake.Calls))
	}
}

// syncGameBotTask upserts a kind = game_bot proof task with the given repo files and an empty hidden
// tarball, the way games.Sync sets up tanks-bot in the real server - this package doesn't depend on
// games, so it builds a minimal one directly.
func syncGameBotTask(t *testing.T, d *dbtest.DB, slug string, repoFiles map[string][]byte) {
	t.Helper()
	repoTar, err := proofs.TarFiles(repoFiles)
	if err != nil {
		t.Fatal(err)
	}
	hiddenTar, err := proofs.TarFiles(nil)
	if err != nil {
		t.Fatal(err)
	}
	task := proofs.Task{
		Slug: slug, Title: "Improve your bot", Language: "python", Kind: proofs.KindGameBot,
		Image: "arena-tanks-bot:1", RunCmd: "true", AgentTimeoutS: 1200, SandboxTimeoutS: 120,
		TaskMD: "task", RepoTar: repoTar, HiddenTar: hiddenTar,
	}
	if err := proofs.SyncCatalog(context.Background(), d.AdminPool, []proofs.Task{task}); err != nil {
		t.Fatal(err)
	}
}

// TestGameBotProofWithoutJudgeIsInfra: RunProof on a game_bot proof without SetGameBotJudge configured
// must not silently treat the proof as an ordinary one - it is a platform error (the job retries) that
// ends in infra_error once retries are exhausted, exactly like a runner that can't reach docker.
func TestGameBotProofWithoutJudgeIsInfra(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	f := setup(t)
	syncGameBotTask(t, f.d, "tanks-bot", map[string][]byte{"README.md": []byte("old\n")})
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")

	p, err := f.proofs.CreateWithRepo(ctx, f.userID, "tanks-bot", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	claimed, _, err := f.proofs.Claim(ctx, f.agent)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	diff := "--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-old\n+new\n"
	if err := f.proofs.SubmitResult(ctx, f.agent, p.ID, proofs.ResultInput{Diff: diff}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	w := proofs.NewWorker(f.d.AppPool, sandbox.PassAll{}, t.TempDir(), log)
	// No SetGameBotJudge call.
	if err := w.RunProof(ctx, p.ID); err == nil {
		t.Fatal("expected an error: no game bot judge is configured")
	}
	got, _ := f.proofs.Get(ctx, f.userID, p.ID)
	if got.Status != proofs.StatusRunningSandbox {
		t.Fatalf("before giving up the proof stays running_sandbox: %+v", got)
	}
	if err := w.MarkInfraError(ctx, p.ID, "no game bot judge configured"); err != nil {
		t.Fatal(err)
	}
	got, _ = f.proofs.Get(ctx, f.userID, p.ID)
	if got.Status != proofs.StatusInfraError || got.FailureReason == "" {
		t.Fatalf("%+v", got)
	}
}
