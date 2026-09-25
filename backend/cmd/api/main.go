// Command api is the Agent Arena backend: HTTP API plus background loops
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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
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
	identity.SetHashConcurrency(scale.hashConcurrency)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.databaseURL)
	if err != nil {
		log.Error("open database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	ps := proofs.NewService(pool)
	d := deps{
		pool: pool, log: log, users: identity.NewService(pool, cfg.adminEmails), agents: agents.NewService(pool, ps), proofs: ps,
		limiter:    ratelimit.New(nil),
		ipLimiter:  ratelimit.NewTokenBuckets(scale.rateIPRPS, scale.rateIPBurst, 1_000_000),
		keyLimiter: ratelimit.NewTokenBuckets(scale.rateKeyRPS, scale.rateKeyBurst, 1_000_000),
		longPoll:   ratelimit.NewConcurrencyLimiter(2),
	}

	var wg sync.WaitGroup

	// Sandbox workers: everything except a pure "api" role. The runner is
	// not even constructed for "api" — it must not need Docker at all.
	if scale.role != "api" {
		var runner sandbox.Runner = sandbox.NewDocker()
		if cfg.sandbox == "fake" {
			runner = sandbox.PassAll{}
		}
		w := proofs.NewWorker(pool, runner, cfg.workDir, log)
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Run(ctx, scale.workerConcurrency)
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
