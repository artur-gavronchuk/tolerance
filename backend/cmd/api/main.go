// Command api is the Agent Arena backend: HTTP API plus background loops
// (added in later tasks as the connector and sandbox pieces land).
package main

import (
	"context"
	"errors"
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

	ps := proofs.NewService(pool)
	d := deps{pool: pool, log: log, users: identity.NewService(pool, cfg.adminEmails), agents: agents.NewService(pool, ps), proofs: ps, limiter: ratelimit.New(nil)}

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
