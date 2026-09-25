package proofs_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/proofs"
)

func TestClaim_ExactlyOneConnectorGetsTheTask(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	p, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	got := make([]*proofs.Proof, 8)
	for i := range got {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], _, _ = f.proofs.Claim(ctx, f.agent)
		}(i)
	}
	wg.Wait()
	n := 0
	for _, g := range got {
		if g != nil {
			n++
			if g.ID != p.ID || g.Status != proofs.StatusClaimed {
				t.Fatalf("claimed wrong thing: %+v", g)
			}
		}
	}
	if n != 1 {
		t.Fatalf("expected exactly one claim, got %d", n)
	}
	if again, _, _ := f.proofs.Claim(ctx, f.agent); again != nil {
		t.Fatalf("queue must be empty after the claim")
	}
}

func TestConnectorFlow_RepoStartedResult(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	if _, err := f.proofs.Create(ctx, f.userID, "go-fix-retry"); err != nil {
		t.Fatal(err)
	}
	p, task, err := f.proofs.Claim(ctx, f.agent)
	if err != nil || p == nil || task.Slug != "go-fix-retry" || task.TaskMD == "" {
		t.Fatalf("claim: %v %+v %+v", err, p, task)
	}
	tarball, err := f.proofs.RepoTar(ctx, f.agent, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(tarball)
	if hex.EncodeToString(sum[:]) != task.RepoSHA256 {
		t.Fatalf("tarball sha mismatch")
	}
	if _, err := f.proofs.RepoTar(ctx, "agent_other", p.ID); err == nil {
		t.Fatalf("another agent must not download this proof's repo")
	}
	if err := f.proofs.Started(ctx, f.agent, p.ID); err != nil {
		t.Fatal(err)
	}
	in := proofs.ResultInput{Diff: "--- a/x\n+++ b/x\n", LogTail: "token=SECRET123456789\ndone\n", DurationMS: 1234, ExitCode: 0}
	if err := f.proofs.SubmitResult(ctx, f.agent, p.ID, in); err != nil {
		t.Fatalf("result: %v", err)
	}
	if err := f.proofs.SubmitResult(ctx, f.agent, p.ID, in); err == nil {
		t.Fatalf("second result must be rejected")
	}
	got, _ := f.proofs.Get(ctx, f.userID, p.ID)
	if got.Status != proofs.StatusDiffSubmitted || got.Diff != in.Diff || strings.Contains(got.AgentLogTail, "SECRET123456789") || *got.AgentDurationMS != 1234 {
		t.Fatalf("stored: %+v", got)
	}
	var jobs int
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind = 'run_proof' AND payload->>'proof_id' = $1`, p.ID).Scan(&jobs)
	})
	if jobs != 1 {
		t.Fatalf("expected one run_proof job, got %d", jobs)
	}
	big := proofs.ResultInput{Diff: strings.Repeat("x", 256<<10+1)}
	if err := f.proofs.SubmitResult(ctx, f.agent, p.ID, big); err == nil {
		t.Fatalf("oversized diff must be rejected")
	}
}

func TestSubmitResult_OversizedDiffFailsTheProof(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	if _, err := f.proofs.Create(ctx, f.userID, "go-fix-retry"); err != nil {
		t.Fatal(err)
	}
	p, _, err := f.proofs.Claim(ctx, f.agent)
	if err != nil || p == nil {
		t.Fatalf("claim: %v %+v", err, p)
	}
	// An oversized submit from an agent that does not own this proof must not
	// touch it: FailOversized and SubmitResult both scope their UPDATE to
	// agent_id, so a stranger's oversized diff matches zero rows.
	if err := f.proofs.FailOversized(ctx, "agent_other", p.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.proofs.SubmitResult(ctx, "agent_other", p.ID, proofs.ResultInput{Diff: strings.Repeat("x", 256<<10+1)}); err == nil {
		t.Fatal("expected an error for a stranger's oversized submit")
	}
	if got, _ := f.proofs.Get(ctx, f.userID, p.ID); got.Status != proofs.StatusClaimed || got.FailureReason != "" {
		t.Fatalf("a stranger's oversized submit must not fail this proof: %+v", got)
	}
	err = f.proofs.SubmitResult(ctx, f.agent, p.ID, proofs.ResultInput{Diff: strings.Repeat("x", 256<<10+1)})
	problem(t, err, 413, "diff_too_large")
	got, _ := f.proofs.Get(ctx, f.userID, p.ID)
	if got.Status != proofs.StatusFailed || got.FailureReason != "diff_too_large" || got.FinishedAt == nil || got.DiffSubmittedAt != nil {
		t.Fatalf("after an oversized diff: %+v", got)
	}
	var jobs int
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE payload->>'proof_id' = $1`, p.ID).Scan(&jobs)
	})
	if jobs != 0 {
		t.Fatalf("an oversized diff must not queue a sandbox run, got %d jobs", jobs)
	}
	// A proof that is no longer running is left alone.
	if err := f.proofs.FailOversized(ctx, f.agent, p.ID); err != nil {
		t.Fatal(err)
	}
}
