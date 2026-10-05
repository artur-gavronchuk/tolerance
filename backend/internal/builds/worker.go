package builds

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/sanitize"
	"tolerance/internal/sandbox"
	"tolerance/internal/submissions"
)

//go:embed runner.py
var runnerPy []byte

const (
	// Image is the sandbox image with Chromium + Playwright (site/Dockerfile).
	Image      = "arena-site:1"
	maxLogTail = 8 << 10
	maxShot    = 2 << 20
	stuckAfter = 20 * time.Minute
)

// Worker scores queued entries one at a time. A nil runner is the fake mode (ARENA_SANDBOX=fake): every
// scenario passes, no quality signals, no screenshot.
type Worker struct {
	pool    *db.Pool
	runner  sandbox.Runner
	workDir string
	log     *slog.Logger
}

func NewWorker(pool *db.Pool, runner sandbox.Runner, workDir string, log *slog.Logger) *Worker {
	return &Worker{pool: pool, runner: runner, workDir: workDir, log: log.With("worker", "builds")}
}

func (w *Worker) Run(ctx context.Context) {
	// One api process runs this loop, so whatever is "running" at start was cut off by a restart.
	_ = w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE build_entries SET status = 'queued' WHERE status = 'running'`)
		return err
	})
	sweep := time.NewTicker(time.Minute)
	defer sweep.Stop()
	for {
		ran, err := w.runOne(ctx)
		if err != nil && ctx.Err() == nil {
			w.log.Error("build run", "err", err)
		}
		if ran {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			_ = w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `UPDATE build_entries SET status = 'infra_error', failure_reason = 'stuck', finished_at = now()
					WHERE status IN ('queued', 'running') AND updated_at < now() - make_interval(secs => $1)`, stuckAfter.Seconds())
				return err
			})
		case <-time.After(2 * time.Second):
		}
	}
}

type claimed struct {
	id, challenge string
	version       int
	zip           []byte
}

func (w *Worker) runOne(ctx context.Context) (bool, error) {
	var c claimed
	err := w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `UPDATE build_entries SET status = 'running'
			WHERE id = (SELECT id FROM build_entries WHERE status = 'queued' ORDER BY updated_at LIMIT 1 FOR UPDATE SKIP LOCKED)
			RETURNING id, challenge, uploads, zip`).Scan(&c.id, &c.challenge, &c.version, &c.zip)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	log := w.log.With("entry_id", c.id)
	if err := w.score(ctx, c); err != nil {
		log.Error("build score", "err", err)
		reason := err.Error()
		if len(reason) > 300 {
			reason = reason[:300]
		}
		return true, w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE build_entries SET status = 'infra_error', failure_reason = $3, finished_at = now()
				WHERE id = $1 AND uploads = $2 AND status = 'running'`, c.id, c.version, reason)
			return err
		})
	}
	return true, nil
}

// score runs the entry in the sandbox and writes its verdict. An error is the platform's (infra_error).
func (w *Worker) score(ctx context.Context, c claimed) error {
	ch := bySlug(c.challenge)
	if ch == nil {
		return errors.New("challenge left the catalog")
	}
	if ch.Scenarios == nil && w.runner != nil {
		return errors.New("no hidden tests configured (ARENA_BUILDS_TESTS_DIR)")
	}
	files, err := submissions.ReadZip(c.zip)
	if err != nil {
		return err
	}
	if w.runner == nil {
		var r runReport
		for i := range ch.Scenarios {
			r.Results = append(r.Results, struct {
				Name   string `json:"name"`
				Passed bool   `json:"passed"`
			}{Name: "scenario " + string(rune('a'+i%26)), Passed: true})
		}
		return w.finish(ctx, c, ch, r, nil, nil, "", "fake sandbox: nothing was run", "")
	}

	dir, err := os.MkdirTemp(w.workDir, "build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for p, body := range files { // ReadZip already cleaned and confined the paths
		target := filepath.Join(dir, "solution", filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return err
		}
	}
	marker := "@@" + idgen.New("verdict") + "@@"
	specDoc := map[string]any{"scenarios": ch.Scenarios, "marker": marker}
	if ch.Mobile() { // try.py mirrors this
		specDoc["viewport"] = 390
		specDoc["shot"] = map[string]any{"width": 390, "height": 844, "scale": 2, "touch": true}
	}
	spec, err := json.Marshal(specDoc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "_run"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "_run", "spec.json"), spec, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "_run", "run.py"), runnerPy, 0o644); err != nil {
		return err
	}
	res, err := w.runner.Run(ctx, sandbox.Request{WorkDir: dir, Image: Image, RunCmd: "python3 _run/run.py", Language: "builds",
		Timeout: time.Duration(ch.TimeoutS) * time.Second, CopyOut: "_out"})
	if err != nil {
		return err
	}
	shot, _ := os.ReadFile(filepath.Join(dir, "_out", "shot.jpg"))
	if len(shot) > maxShot {
		shot = nil
	}
	if res.TimedOut {
		return w.finish(ctx, c, ch, runReport{}, nil, shot, marker, res.Output, "timeout")
	}
	for _, line := range strings.Split(res.Output, "\n") {
		i := strings.Index(line, marker)
		if i < 0 {
			continue
		}
		raw := strings.TrimSpace(line[i+len(marker):])
		var r runReport
		var q struct {
			Quality json.RawMessage `json:"quality"`
		}
		if json.Unmarshal([]byte(raw), &r) == nil && json.Unmarshal([]byte(raw), &q) == nil {
			return w.finish(ctx, c, ch, r, q.Quality, shot, marker, res.Output, "")
		}
	}
	return w.finish(ctx, c, ch, runReport{}, nil, shot, marker, res.Output, "no_results")
}

func (w *Worker) finish(ctx context.Context, c claimed, ch *Challenge, r runReport, rawQuality json.RawMessage, shot []byte, marker, output, reason string) error {
	pts, checks := score(len(ch.Scenarios), r, rawQuality)
	body, err := json.Marshal(checks)
	if err != nil {
		return err
	}
	var kept []string
	for _, l := range strings.Split(output, "\n") {
		if marker == "" || !strings.Contains(l, marker) {
			kept = append(kept, l)
		}
	}
	logTail := sanitize.CleanLog(strings.TrimSpace(strings.Join(kept, "\n")), maxLogTail)
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE build_entries SET status = 'done', score = $3, checks = $4, shot = $5, log_tail = $6,
			failure_reason = NULLIF($7, ''), finished_at = now()
			WHERE id = $1 AND uploads = $2 AND status = 'running'`, c.id, c.version, pts, body, shot, logTail, reason)
		return err
	})
}
