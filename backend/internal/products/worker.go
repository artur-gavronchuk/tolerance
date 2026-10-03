package products

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/platform/sanitize"
	"tolerance/internal/sandbox"
	"tolerance/internal/submissions"
)

//go:embed runner.py
var runnerPy []byte

const resultsMarker = "@@RESULTS@@"

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
		owner: fmt.Sprintf("%s-%d-products", host, os.Getpid())}
}

// Run drains run_product jobs and, in the first loop, sweeps stuck entries every 30 seconds.
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
			if err := w.SweepStuck(ctx); err != nil {
				w.log.Error("sweep stuck entries", "err", err)
			}
		case <-time.After(2 * time.Second):
		}
	}
}

// SweepStuck moves entries that sat queued or running for too long to infra_error.
func (w *Worker) SweepStuck(ctx context.Context) error {
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE product_entries SET status = 'infra_error', failure_reason = $1, finished_at = now()
			WHERE status IN ('queued', 'running') AND created_at < now() - make_interval(secs => $2)`, reasonStuck, stuckAfter.Seconds())
		return err
	})
}

func (w *Worker) handle(ctx context.Context, job *jobs.Job) {
	var p RunPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		_, _ = w.queue.Fail(ctx, job.ID, err)
		return
	}
	log := w.log.With("entry_id", p.EntryID, "job_id", job.ID)
	err := w.RunEntry(ctx, p.EntryID)
	if err == nil {
		if err := w.queue.Complete(ctx, job.ID); err != nil {
			log.Error("jobs complete", "err", err)
		}
		return
	}
	log.Error("run_product", "attempt", job.Attempts, "err", err)
	final, ferr := w.queue.Fail(ctx, job.ID, err)
	if ferr != nil {
		log.Error("jobs fail", "err", ferr)
	}
	if final {
		reason := err.Error()
		if len(reason) > 500 {
			reason = reason[:500]
		}
		_ = w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE product_entries SET status = 'infra_error', failure_reason = $2, finished_at = now()
				WHERE id = $1 AND status IN ('queued', 'running')`, p.EntryID, reason)
			return err
		})
	}
}

// RunEntry scores one entry in the sandbox. An error means the platform could not run it (the job retries);
// every verdict about the participant's code is written to the entry and returns nil.
func (w *Worker) RunEntry(ctx context.Context, id string) error {
	var zipData, scenarios []byte
	var image, command string
	var timeoutS int
	err := w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE product_entries e SET status = 'running'
			FROM product_tasks t
			WHERE e.id = $1 AND t.slug = e.task_slug AND e.status IN ('queued', 'running')
			RETURNING e.zip, t.image, t.command, t.timeout_s, t.scenarios`, id).Scan(&zipData, &image, &command, &timeoutS, &scenarios)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var scs []Scenario
	if err := json.Unmarshal(scenarios, &scs); err != nil || len(scs) == 0 {
		return fmt.Errorf("products: entry %s: task has no scenarios", id)
	}
	files, err := submissions.ReadZip(zipData)
	if err != nil {
		return w.finish(ctx, id, scs, nil, "", "invalid_zip")
	}

	dir, err := os.MkdirTemp(w.workDir, "product-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for p, body := range files { // paths are already cleaned and confined by ReadZip
		target := filepath.Join(dir, "solution", filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return err
		}
	}
	spec, err := json.Marshal(map[string]any{"command": command, "scenarios": scs})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "_scenarios"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "_scenarios", "spec.json"), spec, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "_scenarios", "run.py"), runnerPy, 0o644); err != nil {
		return err
	}

	res, err := w.runner.Run(ctx, sandbox.Request{WorkDir: dir, Image: image, RunCmd: "python3 _scenarios/run.py",
		Language: "products", Timeout: time.Duration(timeoutS) * time.Second})
	if err != nil {
		return err
	}
	if res.TimedOut {
		return w.finish(ctx, id, scs, nil, res.Output, ReasonTimeout)
	}
	passed, ok := parseResults(res.Output)
	if !ok {
		return w.finish(ctx, id, scs, nil, res.Output, ReasonNoResults)
	}
	return w.finish(ctx, id, scs, passed, res.Output, "")
}

// parseResults reads the runner's marker line: scenario name -> passed.
func parseResults(out string) (map[string]bool, bool) {
	for _, line := range strings.Split(out, "\n") {
		i := strings.Index(line, resultsMarker)
		if i < 0 {
			continue
		}
		var rs []ScenarioResult
		if json.Unmarshal([]byte(line[i+len(resultsMarker):]), &rs) != nil {
			continue
		}
		m := make(map[string]bool, len(rs))
		for _, r := range rs {
			m[r.Name] = r.Passed
		}
		return m, true
	}
	return nil, false
}

// finish writes the verdict; only an entry this run moved to running gets one (the stuck sweep may have won).
func (w *Worker) finish(ctx context.Context, id string, scs []Scenario, passed map[string]bool, output, reason string) error {
	results := make([]ScenarioResult, 0, len(scs))
	n := 0
	for _, sc := range scs {
		ok := passed[sc.Name]
		results = append(results, ScenarioResult{Name: sc.Name, Passed: ok})
		if ok {
			n++
		}
	}
	body, err := json.Marshal(results)
	if err != nil {
		return err
	}
	var kept []string
	for _, l := range strings.Split(output, "\n") {
		if !strings.Contains(l, resultsMarker) {
			kept = append(kept, l)
		}
	}
	logTail := sanitize.CleanLog(strings.TrimSpace(strings.Join(kept, "\n")), maxLogTail)
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE product_entries SET status = 'done', failure_reason = NULLIF($2, ''), results = $3, passed = $4,
			total = $5, log_tail = $6, finished_at = now() WHERE id = $1 AND status = 'running'`, id, reason, body, n, len(scs), logTail)
		return err
	})
}
