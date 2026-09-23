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
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: []sandbox.TestResult{{Name: "TestA", Passed: true}}}}
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

	t.Run("CRLF diff still applies", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: []sandbox.TestResult{{Name: "T", Passed: true}}}}
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
