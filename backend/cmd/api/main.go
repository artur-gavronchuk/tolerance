// Command api runs the Agent Arena HTTP service. This file is rebuilt in
// full once the domain modules exist (see Task 9 of
// docs/plans/arena-slice-1-foundation.md); for now it only proves the
// platform layer compiles and serves /healthz.
package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	top := http.NewServeMux()
	top.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           withMiddleware(top, cfg),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Print(err)
		}
	}()

	log.Printf("arena api listening on http://%s", cfg.addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func withMiddleware(next http.Handler, cfg config) http.Handler {
	withRequestID := httpx.WithRequestID(func() string { return idgen.New("req") })(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if !applyCORS(w, r, cfg.webOrigin) {
			return
		}
		withRequestID.ServeHTTP(w, r)
	})
}

func applyCORS(w http.ResponseWriter, r *http.Request, allowedOrigin string) bool {
	origin := r.Header.Get("Origin")
	if origin != "" && origin == allowedOrigin {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, Last-Event-ID")
		w.Header().Set("Access-Control-Max-Age", "600")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}

type config struct {
	addr        string
	databaseURL string
	webOrigin   string
}

func loadConfig() (config, error) {
	cfg := config{
		addr:        env("ARENA_ADDR", "127.0.0.1:8080"),
		databaseURL: os.Getenv("ARENA_APP_DATABASE_URL"),
		webOrigin:   os.Getenv("ARENA_WEB_ORIGIN"),
	}
	host, _, err := net.SplitHostPort(cfg.addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return config{}, errors.New("ARENA_ADDR must be a loopback address in this slice; a reverse proxy is expected in front of it")
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
