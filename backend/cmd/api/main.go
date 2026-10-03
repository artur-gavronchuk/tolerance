// Command api is the tolerance backend: HTTP API plus background loops
// (added in later tasks as the connector and sandbox pieces land).
//
// One binary, three roles (ARENA_ROLE): "all" (default, single-process
// dev/small-deploy behaviour, unchanged), "api" (serves the public HTTP
// mux and metrics only — no sandbox workers, so it never needs Docker) and
// "worker" (runs only the proof workers and metrics — no public HTTP
// listener, so it scales independently against a remote Postgres).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"tolerance/internal/admin"
	"tolerance/internal/agents"
	"tolerance/internal/arena"
	"tolerance/internal/challenges"
	"tolerance/internal/games"
	"tolerance/internal/games/match"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/limits"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
	"tolerance/internal/qualifications"
)

func main() {
	baseLog := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := loadConfig()
	if err != nil {
		baseLog.Error("config", "err", err)
		os.Exit(1)
	}
	scale, err := loadScaleConfig()
	if err != nil {
		baseLog.Error("config", "err", err)
		os.Exit(1)
	}
	log := baseLog.With("role", scale.role)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The sandbox worker connects as arena_worker (least privilege: it never needs sessions, OAuth
	// identities or API key hashes) when ARENA_WORKER_DATABASE_URL is set; every other role, and a worker
	// whose host hasn't been given that variable yet, connects as arena_app exactly as before.
	dbURL := cfg.databaseURL
	if scale.role == "worker" && cfg.workerDatabaseURL != "" {
		dbURL = cfg.workerDatabaseURL
	}
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		log.Error("open database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := checkSchema(ctx, pool); err != nil {
		log.Error("schema check", "err", err)
		os.Exit(1)
	}

	ps := proofs.NewService(pool)

	// The launcher is a plain struct (match.DockerLauncher / match.WithHouse
	// wrap it, they don't touch Docker) — constructing it does no I/O — so
	// it is safe to build in every role, including "api", which needs
	// games.NewService for its HTTP routes (bot upload, leaderboard, match
	// views). Nothing on an HTTP path calls the launcher directly: bot
	// uploads enqueue a check_bot job (games.Service.createVersion) instead
	// of qualifying synchronously. Only games.NewWorker and the proof
	// worker's GameBotJudge (both gated to non-"api" roles below) actually
	// invoke it, which is where Docker is really touched.
	if cfg.noLimits {
		limits.Disable()
		log.Info("quotas off (ARENA_NO_LIMITS)")
	}
	var launcher match.Launcher = match.WithHouse(match.DockerLauncher{Image: cfg.botImage})
	if cfg.sandbox == "fake" {
		launcher = match.WithHouse(match.ProcessLauncher{})
	}
	gamesSvc := games.NewService(pool, ps, launcher, log, games.Config{WorkDir: cfg.workDir})

	// Qualification runs listen for version changes on every role: the heartbeat that reports a new
	// version is served by "api" (and "all"), so the listener must be wired wherever agents.Service is.
	agentsSvc := agents.NewService(pool, ps)
	qs := qualifications.NewService(pool, ps)
	qs.SetMinPool(cfg.skillMinPool)
	as := arena.NewService(pool, cfg.skillMinPool)
	adminSvc := admin.NewService(pool)
	challengesSvc := challenges.NewService(pool, ps)
	challengesSvc.SetLogger(log)
	agentsSvc.SetVersionListener(qs)
	agentsSvc.SetChallengePlacesSource(challengesSvc)
	agentsSvc.SetSkillsSource(qs)

	d := deps{
		pool: pool, log: log, users: identity.NewService(pool, cfg.adminEmails), agents: agentsSvc, proofs: ps, games: gamesSvc, quals: qs, arena: as, admin: adminSvc, challenges: challengesSvc,
		limiter:    ratelimit.New(nil),
		providers:  providersFromConfig(cfg),
		ipLimiter:  ratelimit.NewTokenBuckets(scale.rateIPRPS, scale.rateIPBurst, 1_000_000),
		keyLimiter: ratelimit.NewTokenBuckets(scale.rateKeyRPS, scale.rateKeyBurst, 1_000_000),
		longPoll:   ratelimit.NewConcurrencyLimiter(2),
	}

	var wg sync.WaitGroup

	// Sandbox + game-match workers: everything except a pure "api" role. The
	// sandbox runner is not even constructed for "api" — it must not need
	// Docker at all — and neither the proof worker's GameBotJudge nor the
	// games match worker (both of which do reach Docker, via the launcher
	// above) run there either.
	if scale.role != "api" {
		var runner sandbox.Runner = sandbox.NewDocker()
		if cfg.sandbox == "fake" {
			runner = sandbox.PassAll{}
		}
		w := proofs.NewWorker(pool, runner, cfg.workDir, log)
		w.SetGameBotJudge(gamesSvc)
		// Only roles that have a worker advance qualification runs: the finish hook (after each terminal
		// proof transition) and the stalled-run sweep below both run as arena_worker there. Several
		// replicas may sweep at once; OnProofFinished locks the run row and moves it only from its newest proof.
		// Both hooks run after every terminal proof transition and each ignores
		// the kinds that are not its own, so they compose: a qualification proof
		// advances its run, a challenge proof records its entry's result.
		w.SetFinishListener(finishBoth{qs, challengesSvc})
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Run(ctx, scale.workerConcurrency)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			tick := time.NewTicker(30 * time.Second)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					n, err := qs.SweepStalled(ctx)
					if err != nil {
						log.Error("sweep stalled qualification runs", "err", err)
					}
					if n > 0 {
						log.Info("advanced stalled qualification runs", "count", n)
					}
					// Challenges open and close by the clock on the same tick.
					if err := challengesSvc.Tick(ctx); err != nil {
						log.Error("challenge tick", "err", err)
					}
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			games.NewWorker(gamesSvc, pool, games.WorkerConfig{Interval: cfg.matchInterval, Concurrency: cfg.matchConcurrency}, log).Run(ctx)
		}()
	}

	// Metrics: every role, on its own listener, never the public mux.
	if metricsServer := newMetricsServer(scale, pool, log); metricsServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Info("arena metrics listening", "addr", scale.metricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("metrics serve", "err", err)
			}
		}()
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = metricsServer.Shutdown(shutdownCtx)
		}()
	}

	// Public HTTP: everything except a pure "worker" role.
	if scale.role != "worker" {
		server := &http.Server{
			Addr:    cfg.addr,
			Handler: newHandler(cfg, scale, d),
			// The connector's long-poll can legitimately take up to 25s;
			// WriteTimeout must stay comfortably above that.
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      40 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    64 << 10,
		}
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				log.Error("shutdown", "err", err)
			}
		}()

		log.Info("arena api listening", "addr", cfg.addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve", "err", err)
			wg.Wait()
			os.Exit(1)
		}
	} else {
		<-ctx.Done()
	}

	// Wait for the worker loop(s) and metrics server to actually stop before
	// the deferred pool.Close() above runs, so nothing is still using the
	// pool when it closes.
	wg.Wait()
}

// checkSchema fails fast, with a clear message, when the database predates
// the GitHub/Google sign-in change: 00002_schema.sql was edited in place
// (no new migration number), so a database that already ran goose still has
// the old password_hash-only users table and no user_identities, which
// otherwise surfaces later as confusing 500s on dev login and oauth_failed
// on every OAuth callback.
func checkSchema(ctx context.Context, pool *db.Pool) error {
	var name *string
	if err := pool.Raw().QueryRow(ctx, `SELECT to_regclass('user_identities')`).Scan(&name); err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	if name == nil {
		return errors.New("database schema is out of date: user_identities is missing; recreate the database (make reset)")
	}
	return nil
}

// finishBoth fans a finished proof out to both listeners that care about one.
// Each inspects the proof's kind and ignores what is not its own, so the order
// does not matter; an error from either surfaces, and the worker logs it.
type finishBoth struct {
	quals      *qualifications.Service
	challenges *challenges.Service
}

func (f finishBoth) OnProofFinished(ctx context.Context, proofID string) error {
	if err := f.quals.OnProofFinished(ctx, proofID); err != nil {
		return err
	}
	return f.challenges.OnProofFinished(ctx, proofID)
}
