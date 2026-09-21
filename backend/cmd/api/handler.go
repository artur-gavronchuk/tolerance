package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/arena"
	"tolerance/internal/attempts"
	"tolerance/internal/competitions"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/standings"
	"tolerance/internal/submissions"
)

type deps struct {
	pool         *db.Pool
	verifier     identity.TokenVerifier
	users        *identity.Service
	agents       *agents.Service
	standings    *standings.Service
	competitions *competitions.Service
	attempts     *attempts.Service
	submissions  *submissions.Service
}

// newHandler wires four route groups with distinct authentication:
// public GETs (no auth), /me/* (user JWT), /agent/* (API key),
// /admin/* (user JWT + admin role).
func newHandler(cfg config, d deps) http.Handler {
	public := http.NewServeMux()
	competitions.RegisterPublicRoutes(public, d.competitions)
	agents.RegisterPublicRoutes(public, d.agents)
	standings.RegisterPublicRoutes(public, d.standings)
	submissions.RegisterPublicRoutes(public, d.submissions)
	arena.RegisterPublicRoutes(public)
	public.HandleFunc("GET /api/v1/stats", statsHandler(d.pool))

	me := http.NewServeMux()
	identity.RegisterRoutes(me, d.users, d.agents.MeAgent)
	agents.RegisterMeRoutes(me, d.pool, d.agents)
	submissions.RegisterMeRoutes(me, d.submissions)

	agent := http.NewServeMux()
	agents.RegisterAgentRoutes(agent, d.agents)
	attempts.RegisterAgentRoutes(agent, d.attempts, d.competitions)
	submissions.RegisterAgentRoutes(agent, d.pool, d.submissions)

	admin := http.NewServeMux()
	competitions.RegisterAdminRoutes(admin, d.pool, d.competitions)
	attempts.RegisterAdminRoutes(admin, d.attempts)

	userAuth := identity.RequireUser(d.verifier, d.users)
	api := http.NewServeMux()
	api.Handle("/api/v1/me", userAuth(me))
	api.Handle("/api/v1/me/", userAuth(me))
	api.Handle("/api/v1/agent/", identity.RequireAgent(d.agents)(agent))
	api.Handle("/api/v1/admin/", userAuth(identity.RequireAdmin(admin)))
	api.Handle("/api/v1/", public)

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
	return withMiddleware(top, cfg)
}

func statsHandler(pool *db.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var active, agentsN, scored int
		err := pool.Tx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM competitions WHERE status = 'active'),
				(SELECT count(*) FROM agents),
				(SELECT count(*) FROM submissions WHERE score_status = 'scored')`).Scan(&active, &agentsN, &scored)
		})
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]int{"active_competitions": active, "agents": agentsN, "scored_submissions": scored})
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

func applyCORS(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if origin := r.Header.Get("Origin"); origin != "" && origin == allowed {
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
