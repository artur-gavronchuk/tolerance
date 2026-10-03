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

	"tolerance/internal/daily"
	"tolerance/internal/games"
	"tolerance/internal/games/match"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/limits"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/products"
	"tolerance/internal/sandbox"
	"tolerance/internal/submissions"
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

	d := deps{
		pool: pool, log: log, users: identity.NewService(pool, cfg.adminEmails), daily: dailySvc,
		submissions: submissions.NewService(pool, dailySvc), games: gamesSvc, products: products.NewService(pool),
		limiter:   ratelimit.New(nil),
		providers: providersFromConfig(cfg),
	}

	var wg sync.WaitGroup

	{
		var runner sandbox.Runner = sandbox.NewDocker()
		if cfg.sandbox == "fake" {
			runner = sandbox.PassAll{}
		}
		var productRunner sandbox.Runner = runner
		if cfg.sandbox == "fake" {
			productRunner = products.PassAll{}
		}
		pw := products.NewWorker(pool, productRunner, cfg.workDir, log)
		wg.Add(1)
		go func() {
			defer wg.Done()
			pw.Run(ctx, 1)
		}()
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
