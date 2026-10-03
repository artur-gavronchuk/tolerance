// Command api is the tolerance backend: the HTTP API plus the submissions and tanks workers, all in one
// process.
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

	"tolerance/internal/account"
	adminpkg "tolerance/internal/admin"
	"tolerance/internal/analytics"
	"tolerance/internal/daily"
	"tolerance/internal/fairplay"
	"tolerance/internal/games"
	"tolerance/internal/games/match"
	"tolerance/internal/house"
	"tolerance/internal/identity"
	"tolerance/internal/moderation"
	"tolerance/internal/notify"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/limits"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/profiles"
	"tolerance/internal/recap"
	"tolerance/internal/sandbox"
	"tolerance/internal/submissions"
	"tolerance/internal/uploadlink"
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

	if cfg.noLimits {
		limits.Disable()
		log.Info("quotas off (ARENA_NO_LIMITS)")
	}
	var launcher match.Launcher = match.WithHouse(match.DockerLauncher{Image: cfg.botImage})
	if cfg.sandbox == "fake" {
		launcher = match.WithHouse(match.ProcessLauncher{})
	}
	gamesSvc := games.NewService(pool, launcher, log, games.Config{WorkDir: cfg.workDir})
	dailySvc := daily.NewService(pool)
	recapSvc := recap.NewService(pool, dailySvc)

	analyticsSvc := analytics.NewService(pool)
	analytics.Use(analyticsSvc)

	// House agents (ARENA_HOUSE_AGENTS): platform-run coding agents on each day's task. Off when unset.
	houseAgents, err := house.LoadAgents(cfg.houseAgents)
	if err != nil {
		log.Error("house agents", "err", err)
		os.Exit(1)
	}
	if len(houseAgents) > 0 {
		if err := house.Sync(ctx, pool, houseAgents); err != nil {
			log.Error("house agents", "err", err)
			os.Exit(1)
		}
		for _, a := range houseAgents {
			dailySvc.HouseHandles = append(dailySvc.HouseHandles, a.Handle)
		}
		log.Info("house agents on", "count", len(houseAgents))
	}

	d := deps{
		pool: pool, log: log, users: identity.NewService(pool, cfg.adminEmails), daily: dailySvc,
		submissions: submissions.NewService(pool, dailySvc), games: gamesSvc, admin: adminpkg.NewService(pool), moderation: moderation.NewService(pool), profiles: profiles.NewService(pool),
		uploadLinks: uploadlink.NewService(pool), account: account.NewService(pool), recap: recapSvc, notify: notify.NewService(pool, gamesSvc, recapSvc), analytics: analyticsSvc,
		limiter:   ratelimit.New(nil),
		providers: providersFromConfig(cfg),
	}

	d.fairplay = fairplay.NewService(pool, os.Getenv("ARENA_FAIRPLAY_SECRET"))
	d.submissions.OnUpload = d.fairplay.Observe

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		analyticsSvc.Run(ctx, log)
	}()

	if len(houseAgents) > 0 {
		houseWorker := house.NewWorker(pool, dailySvc, d.submissions, houseAgents, cfg.workDir, log)
		dailySvc.OnAssign = houseWorker.EnqueueDay
		wg.Add(1)
		go func() {
			defer wg.Done()
			houseWorker.Run(ctx)
		}()
	}

	{
		var runner sandbox.Runner = sandbox.NewDocker()
		if cfg.sandbox == "fake" {
			runner = sandbox.PassAll{}
		}
		w := submissions.NewWorker(pool, runner, cfg.workDir, log)
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Run(ctx, 1)
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
			// Uploads are up to 5 MB; leave generous time to read and unzip them.
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      60 * time.Second,
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
