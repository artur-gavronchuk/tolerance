package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/platform/sanitize"
	"tolerance/internal/sandbox"
	"tolerance/internal/tasks"
)

// stuckAfter is how long a submission may sit queued or running before the sweep calls it an infra_error.
const stuckAfter = 15 * time.Minute

// Failure reasons stored in submissions.failure_reason.
const (
	ReasonDoesNotApply  = "diff_does_not_apply"
	ReasonTestFiles     = "touches_test_files"
	ReasonHarness       = "harness_tampering"
	ReasonBuildFailed   = "build_failed"
	ReasonTestsFailed   = "hidden_tests_failed"
	ReasonTestsNotRun   = "tests_did_not_run"
	ReasonTimeout       = "timeout"
	reasonStuck         = "stuck"
	maxInfraReasonBytes = 500
)

type Worker struct {
	pool    *db.Pool
	queue   *jobs.Queue
	runner  sandbox.Runner
	workDir string
	log     *slog.Logger
	owner   string
}

func NewWorker(pool *db.Pool, runner sandbox.Runner, workDir string, log *slog.Logger) *Worker {
	host, _ := os.Hostname()
	return &Worker{pool: pool, queue: jobs.New(pool), runner: runner, workDir: workDir, log: log,
		owner: fmt.Sprintf("%s-%d", host, os.Getpid())}
}

// Run drains run_submission jobs with the given concurrency and, in the first loop, sweeps expired job
// leases and stuck submissions every 30 seconds. It returns when ctx is done.
func (w *Worker) Run(ctx context.Context, concurrency int) {
	if concurrency < 1 {
		concurrency = 1
	}
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

func (w *Worker) loop(ctx context.Context, owner string, maintain bool) {
	var tick *time.Ticker
	if maintain {
		tick = time.NewTicker(30 * time.Second)
		defer tick.Stop()
	}
	for {
		job, err := w.queue.Claim(ctx, owner, []string{JobKind}, 15*time.Minute)
		if err != nil && ctx.Err() == nil {
			w.log.Error("jobs claim", "err", err)
		}
		if job != nil {
			w.handle(ctx, job)
			continue
		}
		var tc <-chan time.Time
		if tick != nil {
			tc = tick.C
		}
		select {
		case <-ctx.Done():
			return
		case <-tc:
			w.sweep(ctx)
		case <-time.After(2 * time.Second):
		}
	}
}

func (w *Worker) sweep(ctx context.Context) {
	if _, err := w.queue.Reclaim(ctx); err != nil {
		w.log.Error("jobs reclaim", "err", err)
	}
	n, err := w.SweepStuck(ctx)
	if err != nil {
		w.log.Error("sweep stuck submissions", "err", err)
	}
	if n > 0 {
		w.log.Info("swept stuck submissions", "count", n)
	}
}

// SweepStuck moves submissions that sat queued or running for too long to infra_error.
func (w *Worker) SweepStuck(ctx context.Context) (int, error) {
	var n int64
	err := w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE submissions SET status = 'infra_error', failure_reason = $1, finished_at = now()
			WHERE status IN ('queued', 'running') AND created_at < now() - make_interval(secs => $2)`, reasonStuck, stuckAfter.Seconds())
		n = tag.RowsAffected()
		return err
	})
	return int(n), err
}

func (w *Worker) handle(ctx context.Context, job *jobs.Job) {
	var p RunPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		_, _ = w.queue.Fail(ctx, job.ID, err)
		return
	}
	log := w.log.With("submission_id", p.SubmissionID, "job_id", job.ID)
	err := w.RunSubmission(ctx, p.SubmissionID)
	if err == nil {
		if err := w.queue.Complete(ctx, job.ID); err != nil {
			log.Error("jobs complete", "err", err)
		}
		return
	}
	log.Error("run_submission", "attempt", job.Attempts, "err", err)
	final, ferr := w.queue.Fail(ctx, job.ID, err)
	if ferr != nil {
		log.Error("jobs fail", "err", ferr)
	}
	if final {
		if err := w.MarkInfraError(ctx, p.SubmissionID, err.Error()); err != nil {
			log.Error("mark infra_error", "err", err)
		}
	}
}

type runInput struct {
	diff, slug, language, image, runCmd string
	timeoutS                            int
	repoTar, hiddenTar                  []byte
}

// RunSubmission executes one sandbox run. A returned error means the platform could not run it (the job
// retries); every verdict about the diff is written to the submission and returns nil.
func (w *Worker) RunSubmission(ctx context.Context, id string) error {
	var in runInput
	err := w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE submissions s SET status = 'running'
			FROM tasks t
			WHERE s.id = $1 AND t.slug = s.task_slug AND s.status IN ('queued', 'running')
			RETURNING s.diff, t.slug, t.language, t.image, t.run_cmd, t.sandbox_timeout_s, t.repo_tar, t.hidden_tar`, id).
			Scan(&in.diff, &in.slug, &in.language, &in.image, &in.runCmd, &in.timeoutS, &in.repoTar, &in.hiddenTar)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		w.log.Info("run_submission: nothing to do", "submission", id)
		return nil
	}
	if err != nil {
		return err
	}

	hidden, err := tasks.HiddenTestNames(in.language, in.hiddenTar)
	if err != nil {
		return err
	}
	if len(hidden) == 0 {
		// A task without hidden tests would pass anything: a platform error, never a silent pass.
		return fmt.Errorf("submissions: task %s has no hidden tests", in.slug)
	}
	if TestFileTouched(in.language, in.diff) {
		return w.finish(ctx, id, in.language, StatusFailed, ReasonTestFiles, hidden, nil)
	}
	if HarnessTampered(in.language, in.diff) {
		return w.finish(ctx, id, in.language, StatusFailed, ReasonHarness, hidden, nil)
	}

	dir, err := os.MkdirTemp(w.workDir, "sub-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := tasks.Untar(in.repoTar, dir); err != nil {
		return err
	}
	if reason := applyDiff(ctx, dir, in.diff); reason != "" {
		return w.finish(ctx, id, in.language, StatusFailed, reason, hidden, nil)
	}
	if err := tasks.Untar(in.hiddenTar, dir); err != nil {
		return err
	}

	res, err := w.runner.Run(ctx, sandbox.Request{WorkDir: dir, Image: in.image, RunCmd: in.runCmd,
		Language: in.language, Timeout: time.Duration(in.timeoutS) * time.Second})
	if err != nil {
		return err
	}
	switch {
	case res.TimedOut:
		return w.finish(ctx, id, in.language, StatusFailed, ReasonTimeout, hidden, &res)
	case len(res.Tests) == 0:
		return w.finish(ctx, id, in.language, StatusFailed, ReasonBuildFailed, hidden, &res)
	case res.ExitCode != 0 || anyFailed(res.Tests):
		return w.finish(ctx, id, in.language, StatusFailed, ReasonTestsFailed, hidden, &res)
	case !allPassed(hidden, res.Tests):
		// Everything that ran passed, but not every hidden test ran: the change narrowed what the test
		// command executes (a TestMain, a -run filter smuggled in via init, ...). Only a run of all
		// hidden tests counts.
		return w.finish(ctx, id, in.language, StatusFailed, ReasonTestsNotRun, hidden, &res)
	}
	return w.finish(ctx, id, in.language, StatusPassed, "", hidden, &res)
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

func anyFailed(tests []sandbox.TestResult) bool {
	for _, t := range tests {
		if !t.Passed {
			return true
		}
	}
	return false
}

// applyDiff applies the person's patch with git; CRLF and a missing trailing newline are tolerated.
// Returns "" on success or a failure reason.
//
// Plain `git apply` (no --unsafe-paths, no --directory) resolves paths relative to cmd.Dir and refuses
// absolute paths, ".." and anything under .git, so a patch cannot write outside dir. It can still create a
// symlink, and the hidden tests are later extracted into dir by following paths, so any symlink in the
// result is refused as well.
func applyDiff(ctx context.Context, dir, diff string) string {
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	if strings.TrimSpace(diff) == "" {
		return ReasonDoesNotApply
	}
	if !strings.HasSuffix(diff, "\n") {
		diff += "\n"
	}
	patch := filepath.Join(dir, "..", filepath.Base(dir)+".patch")
	if err := os.WriteFile(patch, []byte(diff), 0o600); err != nil {
		return ReasonDoesNotApply
	}
	defer os.Remove(patch)
	cmd := exec.CommandContext(ctx, "git", "apply", "--whitespace=nowarn", patch)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return ReasonDoesNotApply
	}
	if hasSymlink(dir) {
		return ReasonDoesNotApply
	}
	return ""
}

// hasSymlink reports whether anything under dir is a symlink (or dir cannot be walked, which is treated
// the same way: refuse rather than guess).
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

// humanLog turns `go test -json` output into the plain text a person expects to read; other output passes
// through unchanged.
func humanLog(language, out string) string {
	if language == "python" {
		return out
	}
	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "{") {
			var ev struct {
				Action string `json:"Action"`
				Output string `json:"Output"`
			}
			if json.Unmarshal([]byte(line), &ev) == nil {
				if ev.Action == "output" {
					b.WriteString(ev.Output)
				}
				continue
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// finish writes the verdict. res is nil when the sandbox never ran. Only a submission this run moved to
// running gets a verdict: if the stuck sweep got there first, this run is stale.
func (w *Worker) finish(ctx context.Context, id, language, status, reason string, hidden []string, res *sandbox.Result) error {
	tests := make([]TestResult, 0, len(hidden))
	passedCount := 0
	var logTail string
	if res != nil {
		ran := make(map[string]bool, len(res.Tests))
		for _, t := range res.Tests {
			ran[t.Name] = t.Passed
		}
		for _, n := range hidden {
			tests = append(tests, TestResult{Name: n, Passed: ran[n]})
			if ran[n] {
				passedCount++
			}
		}
		logTail = sanitize.CleanLog(humanLog(language, res.Output), maxLogTail)
	}
	testsJSON, err := json.Marshal(tests)
	if err != nil {
		return err
	}
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE submissions SET status = $2, failure_reason = NULLIF($3, ''), tests = $4, passed_tests = $5,
			log_tail = $6, finished_at = now() WHERE id = $1 AND status = 'running'`, id, status, reason, testsJSON, passedCount, logTail)
		return err
	})
}

// MarkInfraError is called when the job has used every attempt: the platform failed, not the person's code.
func (w *Worker) MarkInfraError(ctx context.Context, id, reason string) error {
	if len(reason) > maxInfraReasonBytes {
		reason = reason[:maxInfraReasonBytes]
	}
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE submissions SET status = 'infra_error', failure_reason = $2, finished_at = now()
			WHERE id = $1 AND status IN ('queued', 'running')`, id, reason)
		return err
	})
}
