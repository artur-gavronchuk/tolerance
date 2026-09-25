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
