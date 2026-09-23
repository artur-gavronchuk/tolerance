package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/ratelimit"
)

type deps struct {
	pool    *db.Pool
	log     *slog.Logger
	users   *identity.Service
	agents  *agents.Service
	limiter *ratelimit.Limiter
}

func newHandler(cfg config, d deps) http.Handler {
	owner := http.NewServeMux()
	identity.RegisterMeRoute(owner, d.users, d.agents.MeAgent)
	agents.RegisterOwnerRoutes(owner, d.agents)

	connector := http.NewServeMux()
	agents.RegisterConnectorRoutes(connector, d.agents)

	session := identity.RequireSession(d.users)
	api := http.NewServeMux()
	identity.RegisterAuthRoutes(api, d.users, d.limiter, cfg.secureCookies)
	api.Handle("/api/v1/me", session(owner))
	api.Handle("/api/v1/agent", session(owner))
	api.Handle("/api/v1/agent/", session(owner))
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

func withMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	withRequestID := httpx.WithRequestID(func() string { return idgen.New("req") })(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		withRequestID.ServeHTTP(w, r)
	})
}
