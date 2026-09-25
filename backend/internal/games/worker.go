package games

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"tolerance/internal/games/match"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/jobs"
)

// WorkerConfig tunes the ladder worker. A zero WorkerConfig gets sane defaults (see NewWorker).
type WorkerConfig struct {
	Interval    time.Duration // ARENA_MATCH_INTERVAL; 0 -> 20s
	Concurrency int           // ARENA_MATCH_CONCURRENCY; 0 -> 1
}

func withWorkerDefaults(cfg WorkerConfig) WorkerConfig {
	if cfg.Interval == 0 {
		cfg.Interval = 20 * time.Second
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	return cfg
}

// Worker runs the games background loops: cfg.Concurrency goroutines draining run_match and check_bot
// jobs, plus one scheduler loop driving the ladder and the broadcast.
type Worker struct {
	svc   *Service
	queue *jobs.Queue
	cfg   WorkerConfig
	log   *slog.Logger
	owner string
}

func NewWorker(svc *Service, pool *db.Pool, cfg WorkerConfig, log *slog.Logger) *Worker {
	host, _ := os.Hostname()
	return &Worker{
		svc: svc, queue: jobs.New(pool), cfg: withWorkerDefaults(cfg), log: log,
		owner: fmt.Sprintf("%s-%d", host, os.Getpid()),
	}
}

// Run starts cfg.Concurrency job-claiming goroutines and runs the scheduler loop until ctx is done. It
// never crashes the process on an error - every failure is logged and retried on the next tick. Run does
// not return until every claimLoop goroutine has also returned, so a caller that waits on Run (e.g. via a
// sync.WaitGroup, as cmd/api/main.go does) knows no job is still mid-handle - and its deferred pool.Close()
// - before Complete/Fail has written the job's outcome, given handle()'s own bounded grace period on top.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < w.cfg.Concurrency; i++ {
		wg.Add(1)
		owner := fmt.Sprintf("%s-%d", w.owner, i)
		go func() {
			defer wg.Done()
			w.claimLoop(ctx, owner)
		}()
	}
	w.scheduleLoop(ctx)
	wg.Wait()
}

// claimLoop repeatedly claims and runs one run_match or check_bot job at a time, polling every 2s when
// the queue is empty.
func (w *Worker) claimLoop(ctx context.Context, owner string) {
	for {
		if ctx.Err() != nil {
			return
		}
		job, err := w.queue.Claim(ctx, owner, []string{"run_match", "check_bot"}, 10*time.Minute)
		if err != nil && ctx.Err() == nil {
			// A Claim error caused by ctx being cancelled (server shutdown) is expected, not a real
			// failure - the loop exits on its own right after via the ctx.Err() check at the top.
			w.log.Error("games: jobs claim", "err", err)
		}
		if job != nil {
			w.handle(ctx, job)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// handleTimeout bounds the context.WithoutCancel(ctx) used for the queue update and the follow-up platform-
// failure marking below: a leased job must be released (or its match/version unstuck) even when the
// server is shutting down, but a wedged database still can't hang the goroutine forever.
const handleTimeout = 30 * time.Second

func (w *Worker) handle(ctx context.Context, job *jobs.Job) {
	err := w.dispatch(ctx, job)

	// Complete/Fail (and the platform-failure marking below) run on a context detached from ctx and bounded
	// on its own: ctx is the caller's, cancelled the moment the server starts shutting down (SIGTERM), and
	// a job whose work already finished - or whose retry bookkeeping still needs writing - must not be left
	// leased for the rest of its 10-minute lease just because the process is on its way out.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), handleTimeout)
	defer cancel()

	if err == nil {
		if cerr := w.queue.Complete(finishCtx, job.ID); cerr != nil {
			w.log.Error("games: jobs complete", "err", cerr)
		}
		return
	}
	w.log.Error("games: job failed", "kind", job.Kind, "id", job.ID, "attempt", job.Attempts, "err", err)
	final, ferr := w.queue.Fail(finishCtx, job.ID, err)
	if ferr != nil {
		w.log.Error("games: jobs fail", "err", ferr)
	}
	if !final {
		return
	}
	switch job.Kind {
	case "check_bot":
		var payload CheckBotPayload
		if uerr := json.Unmarshal(job.Payload, &payload); uerr == nil {
			if merr := w.svc.markCheckPlatformFailure(finishCtx, payload.VersionID); merr != nil {
				w.log.Error("games: mark version rejected", "err", merr)
			}
		}
	case "run_match":
		var payload struct {
			MatchID string `json:"match_id"`
		}
		if uerr := json.Unmarshal(job.Payload, &payload); uerr == nil {
			if merr := w.svc.markMatchPlatformFailure(finishCtx, payload.MatchID); merr != nil {
				w.log.Error("games: mark match infra_error", "err", merr)
			}
		}
	}
}

func (w *Worker) dispatch(ctx context.Context, job *jobs.Job) error {
	switch job.Kind {
	case "run_match":
		var payload struct {
			MatchID string `json:"match_id"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		return w.svc.RunMatch(ctx, payload.MatchID)
	case "check_bot":
		var payload CheckBotPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}
		_, _, err := w.svc.Qualify(ctx, payload.VersionID)
		return err
	default:
		return fmt.Errorf("games: unknown job kind %q", job.Kind)
	}
}

// scheduleLoop drives the ladder and the broadcast: every 2s ScheduleTick and RefreshBroadcast, every
// minute SweepStuck, every hour PruneReplays and match.RemoveStaleBotContainers. It logs every error and
// keeps going; it returns when ctx is done.
func (w *Worker) scheduleLoop(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	minuteTick := time.NewTicker(time.Minute)
	defer minuteTick.Stop()
	hourTick := time.NewTicker(time.Hour)
	defer hourTick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if _, err := w.svc.ScheduleTick(ctx, w.cfg.Concurrency, w.cfg.Interval); err != nil {
				w.log.Error("games: schedule tick", "err", err)
			}
			if err := w.svc.RefreshBroadcast(ctx); err != nil {
				w.log.Error("games: refresh broadcast", "err", err)
			}
		case <-minuteTick.C:
			if n, err := w.svc.SweepStuck(ctx); err != nil {
				w.log.Error("games: sweep stuck", "err", err)
			} else if n > 0 {
				w.log.Info("games: swept stuck matches", "count", n)
			}
			if n, err := w.svc.sweepFailedChecks(ctx); err != nil {
				w.log.Error("games: sweep failed checks", "err", err)
			} else if n > 0 {
				w.log.Info("games: rejected versions with a failed check_bot job", "count", n)
			}
		case <-hourTick.C:
			if n, err := w.svc.PruneReplays(ctx); err != nil {
				w.log.Error("games: prune replays", "err", err)
			} else if n > 0 {
				w.log.Info("games: pruned replays", "count", n)
			}
			if n, err := match.RemoveStaleBotContainers(ctx); err != nil {
				w.log.Error("games: remove stale bot containers", "err", err)
			} else if n > 0 {
				w.log.Info("games: removed stale bot containers", "count", n)
			}
		}
	}
}
