// Command api is the tolerance backend: the HTTP API plus the proof,
// qualification, challenge and tanks workers, all in one process.
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
	log := baseLog
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.databaseURL)
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

	if cfg.noLimits {
		limits.Disable()
		log.Info("quotas off (ARENA_NO_LIMITS)")
	}
	var launcher match.Launcher = match.WithHouse(match.DockerLauncher{Image: cfg.botImage})
	if cfg.sandbox == "fake" {
		launcher = match.WithHouse(match.ProcessLauncher{})
	}
	gamesSvc := games.NewService(pool, ps, launcher, log, games.Config{WorkDir: cfg.workDir})

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
		limiter:   ratelimit.New(nil),
		providers: providersFromConfig(cfg),
	}

	var wg sync.WaitGroup

	{
		var runner sandbox.Runner = sandbox.NewDocker()
		if cfg.sandbox == "fake" {
			runner = sandbox.PassAll{}
		}
		w := proofs.NewWorker(pool, runner, cfg.workDir, log)
		w.SetGameBotJudge(gamesSvc)
		// Both hooks run after every terminal proof transition and each ignores
		// the kinds that are not its own, so they compose: a qualification proof
		// advances its run, a challenge proof records its entry's result.
		w.SetFinishListener(finishBoth{qs, challengesSvc})
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Run(ctx, 1)
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

	{
		server := &http.Server{
			Addr:    cfg.addr,
			Handler: newHandler(cfg, d),
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
	}

	// Wait for the worker loops to actually stop before
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
