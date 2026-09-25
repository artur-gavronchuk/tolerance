package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/proofs"
)

type deps struct {
	pool      *db.Pool
	log       *slog.Logger
	users     *identity.Service
	agents    *agents.Service
	proofs    *proofs.Service
	limiter   *ratelimit.Limiter
	providers map[string]identity.Provider
}

func newHandler(cfg config, d deps) http.Handler {
	owner := http.NewServeMux()
	identity.RegisterMeRoute(owner, d.users, meAgent(d.agents, d.proofs))
	agents.RegisterOwnerRoutes(owner, d.agents)
	proofs.RegisterOwnerRoutes(owner, d.proofs)

	connector := http.NewServeMux()
	agents.RegisterConnectorRoutes(connector, d.agents)
	proofs.RegisterConnectorRoutes(connector, d.proofs)
	connector.HandleFunc("GET /api/v1/connector/status", connectorStatus(d.agents, d.proofs))

	session := identity.RequireSession(d.users)
	api := http.NewServeMux()
	identity.RegisterAuthRoutes(api, d.users, d.limiter, identity.AuthConfig{Providers: d.providers, PublicURL: cfg.publicURL, DevLogin: cfg.devLogin, Secure: cfg.secureCookies})
	// Public: the owner downloads the connector before having it set up.
	// The exact GET pattern wins over the key-protected /api/v1/connector/ prefix.
	api.HandleFunc("GET /api/v1/connector/download", connectorDownload(cfg.connectorDir))
	api.Handle("/api/v1/me", session(owner))
	api.Handle("/api/v1/agent", session(owner))
	api.Handle("/api/v1/agent/", session(owner))
	api.Handle("/api/v1/proof-tasks", session(owner))
	api.Handle("/api/v1/proofs", session(owner))
	api.Handle("/api/v1/proofs/", session(owner))
	api.Handle("/api/v1/connector/", identity.RequireAgent(d.agents)(connector))

	top := http.NewServeMux()
	top.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := d.pool.Tx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "SELECT 1")
			return err
		}); err != nil {
			httpx.Respond(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unavailable"})
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	top.Handle("/api/", api)
	return withMiddleware(top, d.log)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) { s.status = code; s.ResponseWriter.WriteHeader(code) }

func withMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	withRequestID := httpx.WithRequestID(func() string { return idgen.New("req") })(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		withRequestID.ServeHTTP(sw, r)
		if sw.status >= 500 {
			log.Error("request failed", "method", r.Method, "path", r.URL.Path, "status", sw.status, "request_id", sw.Header().Get("X-Request-Id"))
		}
		log.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
	})
}
