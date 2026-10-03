package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/daily"
	"tolerance/internal/games"
	"tolerance/internal/identity"
	"tolerance/internal/platform/clientip"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/products"
	"tolerance/internal/profiles"
	"tolerance/internal/stacks"
	"tolerance/internal/submissions"
	"tolerance/internal/tasks"
)

type deps struct {
	pool        *db.Pool
	log         *slog.Logger
	users       *identity.Service
	daily       *daily.Service
	submissions *submissions.Service
	games       *games.Service
	products    *products.Service
	stacks      *stacks.Service
	profiles    *profiles.Service
	limiter     *ratelimit.Limiter
	providers   map[string]identity.Provider
}

func newHandler(cfg config, d deps) http.Handler {
	owner := http.NewServeMux()
	identity.RegisterMeRoute(owner, d.users, func(ctx context.Context, userID string) (map[string]any, error) {
		st, err := d.daily.StreakOf(ctx, userID)
		return map[string]any{"streak": st, "can_admin": products.CanAdmin(cfg.devLogin, identity.MustFromContext(ctx).Role)}, err
	})
	submissions.RegisterOwnerRoutes(owner, d.submissions)
	games.RegisterOwnerRoutes(owner, d.games)
	products.RegisterOwnerRoutes(owner, d.products, cfg.devLogin)

	admin := http.NewServeMux()
	games.RegisterAdminRoutes(admin, d.games)

	public := http.NewServeMux()
	daily.RegisterPublicRoutes(public, d.daily, identity.OptionalUserID(d.users), d.submissions.MyDay)
	tasks.RegisterPublicRoutes(public, d.pool)
	stacks.RegisterPublicRoutes(public, d.stacks)
	profiles.RegisterPublicRoutes(public, d.profiles)
	games.RegisterPublicRoutes(public, d.games)
	products.RegisterPublicRoutes(public, d.products, identity.OptionalUserID(d.users))

	session := identity.RequireSession(d.users)
	api := http.NewServeMux()
	identity.RegisterAuthRoutes(api, d.users, d.limiter, identity.AuthConfig{
		Providers: d.providers, PublicURL: cfg.publicURL, DevLogin: cfg.devLogin, Secure: cfg.secureCookies, TrustProxy: cfg.trustProxy,
	})
	// Public: the local `arena tanks new|play` tool is downloadable without an account.
	api.HandleFunc("GET /api/v1/connector/download", connectorDownload(cfg.connectorDir))
	api.Handle("/api/v1/me", session(owner))
	api.Handle("/api/v1/me/tanks", session(owner))
	api.Handle("/api/v1/me/tanks/", session(owner))
	api.Handle("/api/v1/submissions", session(owner))
	api.Handle("/api/v1/submissions/", session(owner))
	api.Handle("/api/v1/products", public)
	api.Handle("/api/v1/products/", public)
	api.Handle("POST /api/v1/products/{slug}/entries", session(owner))
	api.Handle("POST /api/v1/products/start-next", session(owner))
	api.Handle("POST /api/v1/products/{slug}/close", session(owner))
	api.Handle("POST /api/v1/products/{slug}/reopen", session(owner))
	api.Handle("POST /api/v1/product-entries/{id}/vote", session(owner))
	api.Handle("DELETE /api/v1/product-entries/{id}/vote", session(owner))
	api.Handle("GET /api/v1/product-entries/{id}/source", public)
	api.Handle("GET /api/v1/product-entries/{id}/zip", public)
	api.Handle("GET /api/v1/product-entries/{id}/site/{path...}", public)
	api.Handle("/api/v1/daily", public)
	api.Handle("/api/v1/daily/", public)
	api.Handle("/api/v1/days", public)
	api.Handle("/api/v1/leaderboard", public)
	api.Handle("/api/v1/stacks", public)
	api.Handle("/api/v1/users/", public)
	api.Handle("/api/v1/tasks/", public)
	api.Handle("/api/v1/tanks/", public)
	// Starting a tournament now: admins, or anyone signed in when the dev login is on (local runs).
	api.Handle("POST /api/v1/tanks/tournaments", session(adminOrDev(cfg.devLogin)(admin)))

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
	return withMiddleware(top, d.log, cfg.trustProxy)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) { s.status = code; s.ResponseWriter.WriteHeader(code) }

// withMiddleware adds request-id, security headers and one structured log
// line per request. The line never includes query strings, headers,
// cookies, Authorization, or any request/response body content (uploads
// included) — only routing and identity metadata. user_id
// is read from a holder that RequireSession fills in as the
// request is authenticated further down the chain (see identity.ActorLog):
// this handler's own r is a different *http.Request value than the one
// those middlewares attach the actor to, so FromContext here would always
// miss; the holder is the one thing both sides share.
func withMiddleware(next http.Handler, log *slog.Logger, trustProxy bool) http.Handler {
	withRequestID := httpx.WithRequestID(func() string { return idgen.New("req") })(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		// Uploaded sites run as an opaque origin (see products.siteCSP); never let them change state.
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("Origin") == "null" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		sw := &statusWriter{ResponseWriter: w, status: 200}
		actorLog := &identity.ActorLog{}
		r = r.WithContext(identity.WithActorLog(r.Context(), actorLog))
		start := time.Now()
		withRequestID.ServeHTTP(sw, r)
		ms := time.Since(start).Milliseconds()
		route := r.Method + " " + r.URL.Path
		fields := []any{
			"request_id", sw.Header().Get("X-Request-Id"),
			"route", route,
			"status", sw.status,
			"ms", ms,
			"client_ip", clientip.FromRequest(r, trustProxy),
		}
		if actorLog.UserID != "" {
			fields = append(fields, "user_id", actorLog.UserID)
		}
		if sw.status >= 500 {
			log.Error("request failed", fields...)
			return
		}
		log.Info("request", fields...)
	})
}

// adminOrDev lets a request through when its user is an admin, or always when dev is true (ARENA_DEV_LOGIN,
// which is refused next to secure cookies, so it never applies to production).
func adminOrDev(dev bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if dev {
			return next
		}
		return identity.RequireAdmin(next)
	}
}
