package games

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/match"
	"tolerance/internal/games/tanks"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/proofs"
)

// alwaysFailLauncher fails every Launch call unconditionally - it is used to force a platform failure (not
// a bot failure) in RunMatch/Qualify, so the worker's final-job-attempt handling can be exercised without
// depending on any bot process actually starting.
type alwaysFailLauncher struct{}

func (alwaysFailLauncher) Launch(context.Context, match.Spec) (match.Bot, error) {
	return nil, fmt.Errorf("games: alwaysFailLauncher always fails")
}

// blockingLauncher stands in for a real docker/process launch that ignores context cancellation until it
// is done (like Worker.handle's own finishCtx grace period assumes of Complete/Fail): its Launch call
// signals launched once, then blocks on release regardless of ctx, so a test can hold a job "mid-handle"
// for as long as it likes and control exactly when the platform call finishes.
type blockingLauncher struct {
	launched chan struct{}
	release  chan struct{}
}

func (l *blockingLauncher) Launch(context.Context, match.Spec) (match.Bot, error) {
	select {
	case l.launched <- struct{}{}:
	default:
	}
	<-l.release
	return nil, fmt.Errorf("games: blockingLauncher released")
}

func starterArchiveForWorkerTest(t *testing.T) []byte {
	t.Helper()
	files, err := tanks.Starter("python")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := proofs.TarFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

// forceFinalAttempt claims kind's oldest queued job and forces its DB attempts counter up to
// max_attempts, so the next queue.Fail call on it is treated as the last one - simulating a job that has
// already used up every retry, without actually waiting through the real backoff schedule.
func forceFinalAttempt(t *testing.T, d *dbtest.DB, q *jobs.Queue, kind string) *jobs.Job {
	t.Helper()
	ctx := context.Background()
	job, err := q.Claim(ctx, "test-worker", []string{kind}, time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim %s: job=%v err=%v", kind, job, err)
	}
	if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE jobs SET attempts = max_attempts WHERE id = $1`, job.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return job
}

func botRatingForWorkerTest(t *testing.T, d *dbtest.DB, botID string) (mu, sigma float64) {
	t.Helper()
	if err := d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT mu, sigma FROM game_bots WHERE id = $1`, botID).Scan(&mu, &sigma)
	}); err != nil {
		t.Fatal(err)
	}
	return mu, sigma
}

// TestWorkerFinalRunMatchFailureMarksInfraError covers the worker's final-attempt handling for run_match:
// once a job has used every retry, the match it stood for must not be left stuck in queued/running
// forever, and no bot's rating may be touched by a platform failure.
func TestWorkerFinalRunMatchFailureMarksInfraError(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	if err := Sync(ctx, d.AdminPool); err != nil {
		t.Fatal(err)
	}
	svc := NewService(d.AppPool, proofs.NewService(d.AppPool), alwaysFailLauncher{}, slog.Default(), Config{WorkDir: t.TempDir()})
	w := NewWorker(svc, d.AppPool, WorkerConfig{}, slog.Default())

	matchID, err := svc.ScheduleTick(ctx, 5, 0)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if matchID == "" {
		t.Fatal("expected a scheduled match")
	}
	mv, err := svc.Match(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}

	before := map[string][2]float64{}
	for _, p := range mv.Players {
		mu, sigma := botRatingForWorkerTest(t, d, p.BotID)
		before[p.BotID] = [2]float64{mu, sigma}
	}

	job := forceFinalAttempt(t, d, w.queue, "run_match")
	w.handle(ctx, job)

	mv2, err := svc.Match(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if mv2.Status != "infra_error" {
		t.Fatalf("status = %q, want infra_error", mv2.Status)
	}
	for botID, want := range before {
		mu, sigma := botRatingForWorkerTest(t, d, botID)
		if mu != want[0] || sigma != want[1] {
			t.Fatalf("bot %s rating changed by a platform failure: before=%v after=(%v,%v)", botID, want, mu, sigma)
		}
	}
}

// TestWorkerFinalCheckBotFailureRejectsVersion covers the worker's final-attempt handling for check_bot:
// once a job has used every retry, a pending version can't be left pending forever - it is rejected with a
// single 'platform' check, so its owner knows to upload again.
func TestWorkerFinalCheckBotFailureRejectsVersion(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	if err := Sync(ctx, d.AdminPool); err != nil {
		t.Fatal(err)
	}
	us := identity.NewService(d.AppPool, nil)
	svc := NewService(d.AppPool, proofs.NewService(d.AppPool), alwaysFailLauncher{}, slog.Default(), Config{WorkDir: t.TempDir()})
	w := NewWorker(svc, d.AppPool, WorkerConfig{}, slog.Default())

	u, _, err := us.SignIn(ctx, identity.Identity{Provider: "dev", Subject: "worker-test@example.com", Email: "worker-test@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.UploadVersion(ctx, u.ID, starterArchiveForWorkerTest(t))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	job := forceFinalAttempt(t, d, w.queue, "check_bot")
	w.handle(ctx, job)

	var status string
	var checksRaw []byte
	if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT status, checks FROM bot_versions WHERE id = $1`, v.ID).Scan(&status, &checksRaw)
	}); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" {
		t.Fatalf("status = %q, want rejected", status)
	}
	var checks []Check
	if err := json.Unmarshal(checksRaw, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || checks[0].Name != "platform" || checks[0].Passed {
		t.Fatalf("expected a single failing platform check, got %+v", checks)
	}
}

// TestSweepFailedChecksRejectsPendingVersion covers M-3: jobs.Queue.Reclaim can park a check_bot job as
// failed without ever going through this package's own final-attempt handling in worker.handle - Reclaim is
// generic (it has no idea what a check_bot job means) and is only ever called from
// internal/proofs.Worker's own maintenance sweep, against the whole shared jobs table, not from anything in
// this package. A version whose check_bot job dies that way must still get unstuck by the games worker's
// own periodic sweep (sweepFailedChecks), not left pending forever.
func TestSweepFailedChecksRejectsPendingVersion(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	if err := Sync(ctx, d.AdminPool); err != nil {
		t.Fatal(err)
	}
	us := identity.NewService(d.AppPool, nil)
	svc := NewService(d.AppPool, proofs.NewService(d.AppPool), alwaysFailLauncher{}, slog.Default(), Config{WorkDir: t.TempDir()})

	u, _, err := us.SignIn(ctx, identity.Identity{Provider: "dev", Subject: "sweep-test@example.com", Email: "sweep-test@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.UploadVersion(ctx, u.ID, starterArchiveForWorkerTest(t))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	// Simulate what Reclaim does to a leased job that has used every attempt: park it 'failed' directly,
	// without going through worker.handle's final-attempt path.
	if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE jobs SET state = 'failed', attempts = max_attempts
			WHERE kind = 'check_bot' AND payload ->> 'version_id' = $1`, v.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	n, err := svc.sweepFailedChecks(ctx)
	if err != nil {
		t.Fatalf("sweepFailedChecks: %v", err)
	}
	if n != 1 {
		t.Fatalf("swept = %d, want 1", n)
	}

	var status string
	var checksRaw []byte
	if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT status, checks FROM bot_versions WHERE id = $1`, v.ID).Scan(&status, &checksRaw)
	}); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" {
		t.Fatalf("status = %q, want rejected", status)
	}
	var checks []Check
	if err := json.Unmarshal(checksRaw, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || checks[0].Name != "platform" || checks[0].Passed {
		t.Fatalf("expected a single failing platform check, got %+v", checks)
	}

	// Nothing left pending with a failed job, so a second sweep is a no-op.
	if n, err := svc.sweepFailedChecks(ctx); err != nil || n != 0 {
		t.Fatalf("second sweep: n=%d err=%v, want 0, nil", n, err)
	}
}

// TestFinishMatchStoresPlayedTicks covers I-1: a match that ends early by elimination (rather than by
// reaching its configured tick limit) must have finishMatch record how many ticks it actually played, not
// the tick count it was launched with - the live broadcast's duration is derived straight from this column,
// and a stale "always 1200 ticks" value makes the viewer seek past the replay's end and loop.
func TestFinishMatchStoresPlayedTicks(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	if err := Sync(ctx, d.AdminPool); err != nil {
		t.Fatal(err)
	}
	svc := NewService(d.AppPool, proofs.NewService(d.AppPool), alwaysFailLauncher{}, slog.Default(), Config{WorkDir: t.TempDir()})

	matchID := idgen.New("match")
	if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO matches (id, game, kind, status, seed, map, ticks, started_at)
			VALUES ($1, 'tanks', 'ladder', 'running', 1, 'arena', 1200, now())`, matchID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO match_players (match_id, slot, bot_id, version_id)
			VALUES ($1, 0, 'bot_house_hunter', 'bv_house_hunter')`, matchID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO match_players (match_id, slot, bot_id, version_id)
			VALUES ($1, 1, 'bot_house_sniper', 'bv_house_sniper')`, matchID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	participants := []playerInput{
		{BotID: "bot_house_hunter", VersionID: "bv_house_hunter", Name: "Hunter", House: true},
		{BotID: "bot_house_sniper", VersionID: "bv_house_sniper", Name: "Sniper", House: true},
	}

	rules := tanks.DefaultRules()
	frames := make([]tanks.Frame, 51) // ticks 0..50 recorded: the match ended by elimination, not the 1200-tick limit
	for i := range frames {
		frames[i] = tanks.Frame{T: i, K: [][]float64{}, S: [][]float64{}, B: [][]float64{}}
	}
	result := match.Result{
		Replay: tanks.Replay{
			Version: 1, Engine: tanks.EngineVersion, Seed: 1, Map: "arena", TickRate: rules.TickRate,
			Rules: rules, Walls: []tanks.Rect{}, Players: []tanks.ReplayPlayer{{Slot: 0}, {Slot: 1}},
			Frames: frames, Events: []tanks.Event{},
			Result: []tanks.PlayerResult{{Slot: 0, Place: 1, Status: match.StatusOK}, {Slot: 1, Place: 2, Status: match.StatusOK}},
		},
		Players: []match.PlayerOutcome{
			{Slot: 0, Place: 1, Status: match.StatusOK},
			{Slot: 1, Place: 2, Status: match.StatusOK},
		},
	}

	if err := svc.finishMatch(ctx, matchID, participants, result); err != nil {
		t.Fatalf("finishMatch: %v", err)
	}

	var ticks int
	if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT ticks FROM matches WHERE id = $1`, matchID).Scan(&ticks)
	}); err != nil {
		t.Fatal(err)
	}
	if ticks != 50 {
		t.Fatalf("ticks = %d, want 50 (the match ended early)", ticks)
	}

	if err := svc.RefreshBroadcast(ctx); err != nil {
		t.Fatalf("RefreshBroadcast: %v", err)
	}
	live, err := svc.Live(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if live.DurationMS != 50*100 {
		t.Fatalf("broadcast duration_ms = %d, want %d", live.DurationMS, 50*100)
	}
}

// TestWorkerRunWaitsForInFlightJob covers the shutdown race this package's Worker.Run used to have: it
// started claimLoop goroutines with a bare `go` and returned as soon as scheduleLoop saw ctx done, without
// waiting for those goroutines. cmd/api/main.go's wg.Wait() (which gates the deferred pool.Close()) treats
// Run's return as "the games worker is done" - so a job still mid-handle when ctx is cancelled (e.g. a
// docker/process launch that, like blockingLauncher here, does not itself respect ctx) could still be
// writing its Complete/Fail outcome via handle()'s detached finishCtx after the pool was already closed.
// Run must not return until every claimLoop goroutine - and so every in-flight handle() - has finished.
func TestWorkerRunWaitsForInFlightJob(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	if err := Sync(ctx, d.AdminPool); err != nil {
		t.Fatal(err)
	}

	launcher := &blockingLauncher{launched: make(chan struct{}, 1), release: make(chan struct{})}
	svc := NewService(d.AppPool, proofs.NewService(d.AppPool), launcher, slog.Default(), Config{WorkDir: t.TempDir()})
	w := NewWorker(svc, d.AppPool, WorkerConfig{Concurrency: 1}, slog.Default())

	matchID, err := svc.ScheduleTick(ctx, 5, 0)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if matchID == "" {
		t.Fatal("expected a scheduled match")
	}

	runCtx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		w.Run(runCtx)
		close(runDone)
	}()

	// Wait for the run_match job to be claimed and the (fake) platform call to actually start -
	// i.e. the job is mid-handle - before simulating shutdown.
	select {
	case <-launcher.launched:
	case <-time.After(5 * time.Second):
		t.Fatal("launcher was never invoked - the job was never claimed")
	}

	cancel() // simulate SIGTERM: the caller now considers the worker shutting down

	// Run must not return while the job is still mid-handle, even though ctx is already cancelled -
	// scheduleLoop exits immediately on cancel, but claimLoop's in-flight handle() does not.
	select {
	case <-runDone:
		t.Fatal("Run returned while a job was still mid-handle")
	case <-time.After(300 * time.Millisecond):
	}

	close(launcher.release) // let the blocked platform call finish, as if the sandbox/process just exited

	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return soon after the in-flight job finished")
	}

	// By the time Run returned, handle() must already have written the job's outcome (queue.Fail here,
	// since blockingLauncher errors out) - not left it leased for cmd/api's pool.Close() to race against.
	var state string
	var leaseOwner *string
	if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state, lease_owner FROM jobs WHERE kind = 'run_match' AND payload ->> 'match_id' = $1`, matchID).
			Scan(&state, &leaseOwner)
	}); err != nil {
		t.Fatal(err)
	}
	if state != "queued" {
		t.Fatalf("job state = %q, want queued (Fail should have run before Run returned)", state)
	}
	if leaseOwner != nil {
		t.Fatalf("lease_owner = %q, want nil (Fail clears it)", *leaseOwner)
	}
}
