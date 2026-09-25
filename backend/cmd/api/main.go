// Command api is the tolerance backend: HTTP API plus background loops
// (added in later tasks as the connector and sandbox pieces land).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tolerance/internal/agents"
	"tolerance/internal/games"
	"tolerance/internal/games/match"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)
	cfg, err := loadConfig()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
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
	d := deps{pool: pool, log: log, users: identity.NewService(pool, cfg.adminEmails), agents: agents.NewService(pool, ps), proofs: ps,
		limiter: ratelimit.New(nil), providers: providersFromConfig(cfg)}

	var runner sandbox.Runner = sandbox.NewDocker()
	var launcher match.Launcher = match.WithHouse(match.DockerLauncher{Image: cfg.botImage})
	if cfg.sandbox == "fake" {
		runner = sandbox.PassAll{}
		launcher = match.WithHouse(match.ProcessLauncher{})
	}
	worker := proofs.NewWorker(pool, runner, cfg.workDir, log)
	gamesSvc := games.NewService(pool, ps, launcher, log, games.Config{WorkDir: cfg.workDir})
	worker.SetGameBotJudge(gamesSvc)
	d.games = gamesSvc
	go worker.Run(ctx)
	go games.NewWorker(gamesSvc, pool, games.WorkerConfig{Interval: cfg.matchInterval, Concurrency: cfg.matchConcurrency}, log).Run(ctx)

	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           newHandler(cfg, d),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      0, // long-lived responses arrive in a later task
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
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
		os.Exit(1)
	}
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
