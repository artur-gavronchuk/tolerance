// Command api is the whole Agent Arena backend: HTTP API plus the
// background loops (competition auto-close now; arena coordinator and
// judge worker in later slices).
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
	"tolerance/internal/attempts"
	"tolerance/internal/competitions"
	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/db"
	"tolerance/internal/standings"
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

	st := standings.NewService(pool)
	d := deps{
		pool:         pool,
		verifier:     auth.NewVerifier(cfg.oidcIssuer, cfg.oidcAudience, cfg.oidcJWKSURL, 10*time.Minute),
		users:        identity.NewService(pool, cfg.adminEmails),
		agents:       agents.NewService(pool, st),
		standings:    st,
		competitions: competitions.NewService(pool),
		attempts:     attempts.NewService(pool),
	}
	go competitions.RunCloser(ctx, d.competitions, 30*time.Second, log)

	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           newHandler(cfg, d),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      0, // SSE in slice 3 needs long-lived responses
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
