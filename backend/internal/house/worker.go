package house

import (
	"archive/zip"
	"bytes"
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
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/daily"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/submissions"
	"tolerance/internal/tasks"
)

// tickEvery is how often the worker makes sure today's runs are queued (a safety net next to the hook that
// fires when the day's task is first assigned).
const tickEvery = 10 * time.Minute

// Payload is the run_house_agent job payload.
type Payload struct {
	Day    string `json:"day"`
	Handle string `json:"handle"`
}

func dedupeKey(day, handle string) string { return "house:" + day + ":" + handle }

// Sync upserts one house user per configured agent. Idempotent; run by cmd/api on start. A handle already
// taken by a person is an error: rename the house agent in the config.
func Sync(ctx context.Context, pool *db.Pool, agents []Agent) error {
	return pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, a := range agents {
			_, err := tx.Exec(ctx, `
				INSERT INTO users (id, email, handle, role, house, house_name) VALUES ($1, $2, $3, 'user', true, $4)
				ON CONFLICT (id) DO UPDATE SET house = true, house_name = EXCLUDED.house_name`,
				a.UserID(), a.Email(), a.Handle, a.Name)
			var pe *pgconn.PgError
			if errors.As(err, &pe) && pe.Code == "23505" {
				return fmt.Errorf("house: handle %q is taken by another user; rename the house agent", a.Handle)
			}
			if err != nil {
				return fmt.Errorf("house: sync %s: %w", a.Handle, err)
			}
		}
		return nil
	})
}

type Worker struct {
	pool    *db.Pool
	queue   *jobs.Queue
	daily   *daily.Service
	subs    *submissions.Service
	agents  []Agent
	workDir string
	log     *slog.Logger
	owner   string
}

func NewWorker(pool *db.Pool, d *daily.Service, subs *submissions.Service, agents []Agent, workDir string, log *slog.Logger) *Worker {
	host, _ := os.Hostname()
	return &Worker{pool: pool, queue: jobs.New(pool), daily: d, subs: subs, agents: agents, workDir: workDir, log: log,
		owner: fmt.Sprintf("house-%s-%d", host, os.Getpid())}
}

// EnqueueDay queues one run per agent for the day. Idempotent: the dedupe key makes a day's run once-only,
// whatever mix of hook and tick asks.
func (w *Worker) EnqueueDay(ctx context.Context, day string) {
	err := w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, a := range w.agents {
			if _, err := jobs.Enqueue(ctx, tx, JobKind, Payload{Day: day, Handle: a.Handle}, dedupeKey(day, a.Handle)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		w.log.Error("house: enqueue", "day", day, "err", err)
	}
}

// ensureToday makes sure today's task is assigned (so the runs start at UTC midnight, not at the first
// visitor) and queued for every agent.
func (w *Worker) ensureToday(ctx context.Context) {
	day := daily.Today()
	if _, err := w.daily.TaskFor(ctx, day); err != nil {
		if !errors.Is(err, daily.ErrNoTasks) {
			w.log.Error("house: assign today's task", "err", err)
		}
		return
	}
	w.EnqueueDay(ctx, day)
}

// Run drains run_house_agent jobs until ctx is done. Agents run concurrently, one job each; the first loop
// also ticks.
func (w *Worker) Run(ctx context.Context) {
	n := min(len(w.agents), 4)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w.loop(ctx, fmt.Sprintf("%s-%d", w.owner, i), i == 0)
		}(i)
	}
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context, owner string, maintain bool) {
	lease := 15 * time.Minute
	for _, a := range w.agents {
		lease = max(lease, time.Duration(a.TimeoutMin)*time.Minute+15*time.Minute)
	}
	var tick *time.Ticker
	if maintain {
		w.ensureToday(ctx)
		tick = time.NewTicker(tickEvery)
		defer tick.Stop()
	}
	for {
		job, err := w.queue.Claim(ctx, owner, []string{JobKind}, lease)
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
			w.ensureToday(ctx)
		case <-time.After(5 * time.Second):
		}
	}
}

func (w *Worker) handle(ctx context.Context, job *jobs.Job) {
	var p Payload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		_, _ = w.queue.Fail(ctx, job.ID, err)
		return
	}
	log := w.log.With("job_id", job.ID, "day", p.Day, "agent", p.Handle)
	err := w.runAgent(ctx, log, p)
	if err == nil {
		if err := w.queue.Complete(ctx, job.ID); err != nil {
			log.Error("jobs complete", "err", err)
		}
		return
	}
	log.Error("run_house_agent", "attempt", job.Attempts, "err", err)
	if _, ferr := w.queue.Fail(ctx, job.ID, err); ferr != nil {
		log.Error("jobs fail", "err", ferr)
	}
}

func (w *Worker) agent(handle string) (Agent, bool) {
	for _, a := range w.agents {
		if a.Handle == handle {
			return a, true
		}
	}
	return Agent{}, false
}

// runAgent runs one agent on one day's task and submits what it left behind. A returned error means the
// platform failed (the job retries, bounded by the queue's max attempts); whatever the agent did, good or
// bad, is a verdict, never an error.
func (w *Worker) runAgent(ctx context.Context, log *slog.Logger, p Payload) error {
	a, ok := w.agent(p.Handle)
	if !ok {
		log.Info("house: agent no longer configured, skipping")
		return nil
	}
	if p.Day != daily.Today() {
		log.Info("house: day is over, skipping")
		return nil
	}
	// One shot a day: a result already stored means a crashed earlier attempt got as far as submitting.
	var have int
	if err := w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM submissions WHERE user_id = $1 AND day = $2::date AND status <> 'infra_error'`,
			a.UserID(), p.Day).Scan(&have)
	}); err != nil {
		return err
	}
	if have > 0 {
		log.Info("house: already has a result today, skipping")
		return nil
	}

	slug, err := w.daily.TaskFor(ctx, p.Day)
	if err != nil {
		return err
	}
	var repoTar []byte
	var taskMD, kind string
	var direction *string
	if err := w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT repo_tar, task_md, kind, direction FROM tasks WHERE slug = $1`, slug).Scan(&repoTar, &taskMD, &kind, &direction)
	}); err != nil {
		return err
	}

	dir, err := os.MkdirTemp(w.workDir, "house-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	// The same files people get in repo.zip: the repo plus TASK.md.
	if err := tasks.Untar(repoTar, dir); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, tasks.TaskFile)); errors.Is(err, fs.ErrNotExist) && taskMD != "" {
		if err := os.WriteFile(filepath.Join(dir, tasks.TaskFile), []byte(taskMD), 0o644); err != nil {
			return err
		}
	}

	started := time.Now()
	logTail, runErr := runCommand(ctx, dir, a, Prompt(kind, direction))
	if ctx.Err() != nil {
		return ctx.Err() // shutting down: leave the job to be retried
	}
	log.Info("house: agent finished", "task", slug, "seconds", int(time.Since(started).Seconds()), "result", runErr, "log_tail", logTail)

	if daily.Today() != p.Day {
		log.Info("house: day ended while the agent worked, discarding the result")
		return nil
	}
	zipped, err := zipDir(dir)
	if err != nil {
		return err
	}
	_, err = w.subs.Create(ctx, a.UserID(), "", "solution.zip", zipped, a.MadeWith)
	var pe *httpx.Problem
	if errors.As(err, &pe) && pe.Status >= 400 && pe.Status < 500 {
		// No changes, unreadable zip, too large a diff: the agent's output is what it is, record a 0.
		log.Info("house: no gradable result", "why", pe.Message)
		return w.subs.RecordNoResult(ctx, a.UserID(), a.MadeWith, "No gradable result: "+pe.Message+"\n\n"+logTail)
	}
	return err
}

// runCommand runs the agent's command with `sh -c` in dir under its timeout, with a scrubbed environment.
// It returns the tail of the combined output and how the run ended ("ok", "exit status 1", "timeout").
func runCommand(ctx context.Context, dir string, a Agent, prompt string) (string, string) {
	cmd := exec.Command("sh", "-c", a.Command)
	cmd.Dir = dir
	cmd.Env = append(scrubbedEnv(), "ARENA_PROMPT="+prompt)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // so a timeout can kill the agent's children too
	cmd.WaitDelay = 5 * time.Second
	tail := &tailBuffer{max: 16 << 10}
	cmd.Stdout, cmd.Stderr = tail, tail
	if err := cmd.Start(); err != nil {
		return err.Error(), "start failed"
	}
	kill := func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(time.Duration(a.TimeoutMin) * time.Minute)
	defer timer.Stop()
	result := "ok"
	select {
	case err := <-done:
		if err != nil {
			result = err.Error()
		}
	case <-timer.C:
		kill()
		<-done
		result = "timeout"
	case <-ctx.Done():
		kill()
		<-done
		result = "cancelled"
	}
	return tail.String(), result
}

// scrubbedEnv is the API's environment without anything the platform keeps secret: every ARENA_* variable
// (database URLs, OAuth secrets) and POSTGRES_*. The agent CLIs keep their own credentials in HOME or in
// ANTHROPIC_*/OPENAI_* variables, which stay.
func scrubbedEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "ARENA_") || strings.HasPrefix(kv, "POSTGRES_") || strings.HasPrefix(kv, "PG") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-t.max:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.ToValidUTF8(string(t.buf), "?")
}

// skipDirs hold things an agent leaves next to the code that are not part of the solution.
var skipDirs = map[string]bool{".git": true, ".claude": true, ".codex": true, ".gemini": true, ".cursor": true, ".aider.tags.cache.v4": true}

// zipDir zips dir's regular files the way a person would zip the repository, leaving out version control and
// the agent CLIs' own state.
func zipDir(dir string) ([]byte, error) {
	var names []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			names = append(names, filepath.ToSlash(rel))
		}
		return nil // symlinks and other specials are dropped
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		f, err := os.Open(filepath.Join(dir, filepath.FromSlash(n)))
		if err != nil {
			return nil, err
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Deflate, Modified: time.Unix(0, 0).UTC()})
		if err == nil {
			_, err = io.Copy(w, f)
		}
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
