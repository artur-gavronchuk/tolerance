package proofs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/proofs/sandbox"
)

type Worker struct {
	pool    *db.Pool
	svc     *Service
	queue   *jobs.Queue
	runner  sandbox.Runner
	workDir string
	log     *slog.Logger
	owner   string
}

func NewWorker(pool *db.Pool, runner sandbox.Runner, workDir string, log *slog.Logger) *Worker {
	host, _ := os.Hostname()
	return &Worker{pool: pool, svc: NewService(pool), queue: jobs.New(pool), runner: runner, workDir: workDir, log: log,
		owner: fmt.Sprintf("%s-%d", host, os.Getpid())}
}

// Run processes run_proof jobs one at a time and, every 30 seconds, reclaims
// dead leases and expires stale proofs. It returns when ctx is done.
func (w *Worker) Run(ctx context.Context) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		job, err := w.queue.Claim(ctx, w.owner, []string{"run_proof"}, 15*time.Minute)
		if err != nil {
			w.log.Error("jobs claim", "err", err)
		}
		if job != nil {
			w.handle(ctx, job)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if _, err := w.queue.Reclaim(ctx); err != nil {
				w.log.Error("jobs reclaim", "err", err)
			}
			if n, err := w.svc.ExpireStale(ctx); err != nil {
				w.log.Error("expire proofs", "err", err)
			} else if n > 0 {
				w.log.Info("expired proofs", "count", n)
			}
		case <-time.After(2 * time.Second):
		}
	}
}

func (w *Worker) handle(ctx context.Context, job *jobs.Job) {
	var payload RunProofPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		_, _ = w.queue.Fail(ctx, job.ID, err)
		return
	}
	err := w.RunProof(ctx, payload.ProofID)
	if err == nil {
		if err := w.queue.Complete(ctx, job.ID); err != nil {
			w.log.Error("jobs complete", "err", err)
		}
		return
	}
	w.log.Error("run_proof", "proof", payload.ProofID, "attempt", job.Attempts, "err", err)
	final, ferr := w.queue.Fail(ctx, job.ID, err)
	if ferr != nil {
		w.log.Error("jobs fail", "err", ferr)
	}
	if final {
		if err := w.MarkInfraError(ctx, payload.ProofID, err.Error()); err != nil {
			w.log.Error("mark infra_error", "err", err)
		}
	}
}

type runInput struct {
	diff     string
	task     Task
	repoTar  []byte
	hiddenTr []byte
}

// RunProof executes one sandbox run. A returned error means the platform
// could not run it (the job will retry); every verdict about the diff is
// written to the proof and returns nil.
func (w *Worker) RunProof(ctx context.Context, proofID string) error {
	var in runInput
	err := w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE proofs p SET status = 'running_sandbox' FROM proof_tasks t
			WHERE p.id = $1 AND p.status IN ('diff_submitted', 'running_sandbox') AND t.slug = p.task_slug
			RETURNING p.diff, t.slug, t.image, t.run_cmd, t.sandbox_timeout_s, t.repo_tar, t.hidden_tar`, proofID).
			Scan(&in.diff, &in.task.Slug, &in.task.Image, &in.task.RunCmd, &in.task.SandboxTimeoutS, &in.repoTar, &in.hiddenTr)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		w.log.Info("run_proof: nothing to do", "proof", proofID)
		return nil
	}
	if err != nil {
		return err
	}

	dir, err := os.MkdirTemp(w.workDir, "proof-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := Untar(in.repoTar, dir); err != nil {
		return err
	}
	if reason := applyDiff(ctx, dir, in.diff); reason != "" {
		return w.finish(ctx, proofID, StatusFailed, reason, nil)
	}
	if err := Untar(in.hiddenTr, dir); err != nil {
		return err
	}

	res, err := w.runner.Run(ctx, sandbox.Request{WorkDir: dir, Image: in.task.Image, RunCmd: in.task.RunCmd,
		Timeout: time.Duration(in.task.SandboxTimeoutS) * time.Second})
	if err != nil {
		return err
	}
	sr := &SandboxResult{Tests: res.Tests, ExitCode: res.ExitCode, Output: res.Output, TimedOut: res.TimedOut}
	switch {
	case res.TimedOut:
		return w.finish(ctx, proofID, StatusFailed, "timeout", sr)
	case len(res.Tests) == 0:
		return w.finish(ctx, proofID, StatusFailed, "build_failed", sr)
	case res.ExitCode != 0 || anyFailed(res.Tests):
		return w.finish(ctx, proofID, StatusFailed, "tests_failed", sr)
	}
	return w.finish(ctx, proofID, StatusPassed, "", sr)
}

func anyFailed(tests []sandbox.TestResult) bool {
	for _, t := range tests {
		if !t.Passed {
			return true
		}
	}
	return false
}

// applyDiff applies the participant's patch with git; CRLF and missing
// trailing newlines are tolerated. Returns "" on success or a failure
// reason. Binary patches are refused by git, which is what we want.
func applyDiff(ctx context.Context, dir, diff string) string {
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	if strings.TrimSpace(diff) == "" {
		return "empty_diff"
	}
	if !strings.HasSuffix(diff, "\n") {
		diff += "\n"
	}
	patch := filepath.Join(dir, "..", filepath.Base(dir)+".patch")
	if err := os.WriteFile(patch, []byte(diff), 0o600); err != nil {
		return "diff_not_applicable"
	}
	defer os.Remove(patch)
	// dir may reach us through a symlinked temp root (e.g. macOS's /tmp ->
	// /private/tmp); git apply's --directory safety check rejects an
	// affected file "beyond a symbolic link" in that case, so resolve to the
	// real path before invoking it. Same directory either way.
	realDir := dir
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		realDir = resolved
	}
	cmd := exec.CommandContext(ctx, "git", "apply", "--whitespace=nowarn", "--unsafe-paths", "--directory", realDir, patch)
	cmd.Dir = realDir
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = out
		return "diff_not_applicable"
	}
	return ""
}

func (w *Worker) finish(ctx context.Context, proofID, status, reason string, sr *SandboxResult) error {
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = $2, failure_reason = $3, sandbox_result = $4, finished_at = now() WHERE id = $1`,
			proofID, status, reason, sr)
		return err
	})
}

// MarkInfraError is called when the job has used every attempt.
func (w *Worker) MarkInfraError(ctx context.Context, proofID, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'infra_error', failure_reason = $2, finished_at = now()
			WHERE id = $1 AND status = 'running_sandbox'`, proofID, reason)
		return err
	})
}
