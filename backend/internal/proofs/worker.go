package proofs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/platform/metrics"
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

// Run starts concurrency claim/handle loops, each with its own lease owner
// so jobs.Claim's FOR UPDATE SKIP LOCKED hands each a distinct job, and, in
// exactly one of them, the 30-second maintenance sweep (Reclaim then
// ExpireStale). The sweep itself is additionally guarded by a Postgres
// advisory lock (see runMaintenance) so that across N worker PROCESSES —
// separate replicas, not just goroutines in this one — only one of them
// ever runs it on a given tick; running it from every process would
// otherwise just be redundant work, never incorrect, since Reclaim and
// ExpireStale are themselves idempotent. Run returns once every loop has
// exited, which happens when ctx is done.
func (w *Worker) Run(ctx context.Context, concurrency int) {
	if concurrency < 1 {
		concurrency = 1
	}
	metrics.WorkerConcurrency.Set(float64(concurrency))
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w.loop(ctx, fmt.Sprintf("%s-%d", w.owner, i), i == 0)
		}(i)
	}
	wg.Wait()
}

// loop repeatedly claims and handles run_proof jobs under owner. When
// maintain is true, it also runs the maintenance sweep on a 30s ticker
// whenever it is otherwise idle.
func (w *Worker) loop(ctx context.Context, owner string, maintain bool) {
	var tick *time.Ticker
	if maintain {
		tick = time.NewTicker(30 * time.Second)
		defer tick.Stop()
	}
	for {
		job, err := w.queue.Claim(ctx, owner, []string{"run_proof"}, 15*time.Minute)
		if err != nil {
			w.log.Error("jobs claim", "err", err)
		}
		if job != nil {
			metrics.WorkersBusy.Inc()
			w.handle(ctx, job)
			metrics.WorkersBusy.Dec()
			continue
		}
		if !maintain {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			w.runMaintenance(ctx)
		case <-time.After(2 * time.Second):
		}
	}
}

// maintenanceLockID is an arbitrary, fixed Postgres advisory-lock id shared
// by every worker process in this codebase, distinct from goose's own
// migration lock id, so pg_try_advisory_lock below can never collide with
// db.Migrate's session lock.
const maintenanceLockID int64 = 7_282_109_355

// runMaintenance reclaims dead job leases and expires stale proofs, but
// only after winning a cluster-wide, non-blocking advisory lock: with
// several worker processes (or several "all"-role replicas) each running
// their own 30s ticker, this keeps exactly one of them doing the sweep on
// any given tick instead of all of them racing the same UPDATEs.
func (w *Worker) runMaintenance(ctx context.Context) {
	conn, err := w.pool.Raw().Acquire(ctx)
	if err != nil {
		w.log.Error("maintenance: acquire connection", "err", err)
		return
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, maintenanceLockID).Scan(&locked); err != nil {
		w.log.Error("maintenance: try lock", "err", err)
		return
	}
	if !locked {
		return // another worker process is already sweeping this tick
	}
	defer func() {
		// Use a detached context: the tick's ctx may already be past its
		// deadline (it never is here, but be defensive), and the unlock must
		// still happen so the next process's try-lock is not stuck behind it.
		if _, err := conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, maintenanceLockID); err != nil {
			w.log.Error("maintenance: unlock", "err", err)
		}
	}()
	if _, err := w.queue.Reclaim(ctx); err != nil {
		w.log.Error("jobs reclaim", "err", err)
	}
	if n, err := w.svc.ExpireStale(ctx); err != nil {
		w.log.Error("expire proofs", "err", err)
	} else if n > 0 {
		w.log.Info("swept stale proofs", "count", n)
	}
}

func (w *Worker) handle(ctx context.Context, job *jobs.Job) {
	var payload RunProofPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		_, _ = w.queue.Fail(ctx, job.ID, err)
		return
	}
	log := w.log.With("proof_id", payload.ProofID, "job_id", job.ID)
	err := w.RunProof(ctx, payload.ProofID)
	if err == nil {
		if err := w.queue.Complete(ctx, job.ID); err != nil {
			log.Error("jobs complete", "err", err)
		}
		return
	}
	log.Error("run_proof", "attempt", job.Attempts, "err", err)
	final, ferr := w.queue.Fail(ctx, job.ID, err)
	if ferr != nil {
		log.Error("jobs fail", "err", ferr)
	}
	if final {
		if err := w.MarkInfraError(ctx, payload.ProofID, err.Error()); err != nil {
			log.Error("mark infra_error", "err", err)
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
	hidden, err := hiddenTestNames(in.hiddenTr)
	if err != nil {
		return err
	}
	if err := Untar(in.repoTar, dir); err != nil {
		return err
	}
	if diffTouchesTestFiles(in.diff) {
		return w.finish(ctx, proofID, StatusFailed, "test_file_modified", nil)
	}
	if reason := applyDiff(ctx, dir, in.diff); reason != "" {
		return w.finish(ctx, proofID, StatusFailed, reason, nil)
	}
	if err := Untar(in.hiddenTr, dir); err != nil {
		return err
	}

	runStart := time.Now()
	res, err := w.runner.Run(ctx, sandbox.Request{WorkDir: dir, Image: in.task.Image, RunCmd: in.task.RunCmd,
		Timeout: time.Duration(in.task.SandboxTimeoutS) * time.Second})
	metrics.SandboxRunSeconds.Observe(time.Since(runStart).Seconds())
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
	case !allPassed(hidden, res.Tests):
		// Everything that ran passed, but not every hidden test ran: the
		// change narrowed what go test executes (a TestMain, a -run filter
		// smuggled in via init, ...). Only a run of all hidden tests counts.
		return w.finish(ctx, proofID, StatusFailed, "hidden_test_missing_or_failed", sr)
	}
	return w.finish(ctx, proofID, StatusPassed, "", sr)
}

// allPassed reports whether every named test appears in tests as passed.
func allPassed(names []string, tests []sandbox.TestResult) bool {
	passed := make(map[string]bool, len(tests))
	for _, t := range tests {
		if t.Passed {
			passed[t.Name] = true
		}
	}
	for _, n := range names {
		if !passed[n] {
			return false
		}
	}
	return true
}

var goTestFunc = regexp.MustCompile(`(?m)^func (Test\w+)\(`)

// hiddenTestNames lists the top-level test functions in the hidden-tests
// tarball, which comes from the server's own catalog and is never touched by
// the participant. Go-specific parsing; extend when a pytest task is added.
func hiddenTestNames(hiddenTar []byte) ([]string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(hiddenTar))
	if err != nil {
		return nil, fmt.Errorf("proofs: hidden tests: %w", err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("proofs: hidden tests: %w", err)
		}
		if h.Typeflag != tar.TypeReg || !strings.HasSuffix(h.Name, ".go") {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(tr, 16<<20))
		if err != nil {
			return nil, fmt.Errorf("proofs: hidden tests: %w", err)
		}
		for _, m := range goTestFunc.FindAllStringSubmatch(string(body), -1) {
			if m[1] != "TestMain" {
				names = append(names, m[1])
			}
		}
	}
	return names, nil
}

// testFileHeaders are the patch lines that name a file the patch touches.
var testFileHeaders = []string{"diff --git ", "--- ", "+++ ", "rename from ", "rename to ", "copy from ", "copy to "}

// diffTouchesTestFiles reports whether the patch adds, edits, deletes or
// renames a Go test file. Participants fix the code, not the tests: editing
// a visible test or adding a TestMain could weaken what the sandbox checks.
func diffTouchesTestFiles(diff string) bool {
	for _, line := range strings.Split(diff, "\n") {
		for _, prefix := range testFileHeaders {
			if strings.HasPrefix(line, prefix) && strings.Contains(line, "_test.go") {
				return true
			}
		}
	}
	return false
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
// reason. Binary patches without --binary data are refused by git.
//
// Plain `git apply` (no --unsafe-paths, no --directory) resolves paths
// relative to cmd.Dir and refuses absolute paths, ".." and anything under
// .git, so a patch cannot write outside dir. It can still create a symlink,
// and the hidden tests are later extracted into dir by following paths, so
// any symlink in the result is refused as well.
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
	cmd := exec.CommandContext(ctx, "git", "apply", "--whitespace=nowarn", patch)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return "diff_not_applicable"
	}
	if hasSymlink(dir) {
		return "diff_not_applicable"
	}
	return ""
}

// hasSymlink reports whether anything under dir is a symlink (or dir cannot
// be walked, which is treated the same way: refuse rather than guess).
func hasSymlink(dir string) bool {
	found := false
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found || err != nil
}

func (w *Worker) finish(ctx context.Context, proofID, status, reason string, sr *SandboxResult) error {
	metrics.ProofVerdicts.WithLabelValues(status).Inc()
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Only a proof this run moved to running_sandbox gets a verdict: if the
		// stuck-proof sweep or a retry got there first, this run is stale.
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = $2, failure_reason = $3, sandbox_result = $4, finished_at = now()
			WHERE id = $1 AND status = 'running_sandbox'`,
			proofID, status, reason, sr)
		return err
	})
}

// MarkInfraError is called when the job has used every attempt.
func (w *Worker) MarkInfraError(ctx context.Context, proofID, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	metrics.ProofVerdicts.WithLabelValues(StatusInfraError).Inc()
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'infra_error', failure_reason = $2, finished_at = now()
			WHERE id = $1 AND status = 'running_sandbox'`, proofID, reason)
		return err
	})
}
