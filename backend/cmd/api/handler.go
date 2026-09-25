package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/games"
	"tolerance/internal/identity"
	"tolerance/internal/platform/clientip"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/metrics"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/proofs"
)

type deps struct {
	pool      *db.Pool
	log       *slog.Logger
	users     *identity.Service
	agents    *agents.Service
	proofs    *proofs.Service
	games     *games.Service
	limiter   *ratelimit.Limiter
	providers map[string]identity.Provider

	// Global request-rate ceiling (see ratelimit_middleware.go), separate
	// from limiter's fixed-window business rules above.
	ipLimiter  *ratelimit.TokenBuckets
	keyLimiter *ratelimit.TokenBuckets
	longPoll   *ratelimit.ConcurrencyLimiter
}

func newHandler(cfg config, scale scaleConfig, d deps) http.Handler {
	owner := http.NewServeMux()
	identity.RegisterMeRoute(owner, d.users, meAgent(d.agents, d.proofs))
	agents.RegisterOwnerRoutes(owner, d.agents)
	proofs.RegisterOwnerRoutes(owner, d.proofs)
	games.RegisterOwnerRoutes(owner, d.games)

	connector := http.NewServeMux()
	agents.RegisterConnectorRoutes(connector, d.agents)
	proofs.RegisterConnectorRoutes(connector, d.proofs)
	games.RegisterConnectorRoutes(connector, d.games)
	connector.HandleFunc("GET /api/v1/connector/status", connectorStatus(d.agents, d.proofs))

	public := http.NewServeMux()
	games.RegisterPublicRoutes(public, d.games)

	session := identity.RequireSession(d.users)
	api := http.NewServeMux()
	identity.RegisterAuthRoutes(api, d.users, d.limiter, identity.AuthConfig{
		Providers: d.providers, PublicURL: cfg.publicURL, DevLogin: cfg.devLogin, Secure: cfg.secureCookies, TrustProxy: scale.trustProxy,
	})
	// Public: the owner downloads the connector before having it set up.
	// The exact GET pattern wins over the key-protected /api/v1/connector/ prefix.
	api.HandleFunc("GET /api/v1/connector/download", connectorDownload(cfg.connectorDir))
	api.Handle("/api/v1/me", session(owner))
	// businessLimits reads the session actor attached by session above, so it
	// wraps owner from the inside, not the whole api mux from the outside.
	limited := businessLimits(d.limiter, owner)
	api.Handle("/api/v1/agent", session(limited))
	api.Handle("/api/v1/agent/", session(limited))
	api.Handle("/api/v1/proof-tasks", session(owner))
	api.Handle("/api/v1/proofs", session(limited))
	api.Handle("/api/v1/proofs/", session(limited))
	api.Handle("/api/v1/me/tanks", session(owner))
	api.Handle("/api/v1/me/tanks/", session(owner))
	api.Handle("/api/v1/tanks/", public)
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
	// /healthz above is mounted outside "/api/" and so is naturally exempt
	// from globalRateLimit, which only wraps the api mux.
	top.Handle("/api/", globalRateLimit(scale, d.ipLimiter, d.keyLimiter, d.longPoll, api))
	// metrics.HTTPMiddleware must wrap top directly (no other r.WithContext
	// hop in between) so it reads the real, routed r.Pattern; see its doc
	// comment and withMiddleware's for why.
	return withMiddleware(metrics.HTTPMiddleware(top), d.log, scale.trustProxy)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) { s.status = code; s.ResponseWriter.WriteHeader(code) }

// isLongPollRoute matches the connector's long-poll route by its matched
// pattern (not a raw path prefix, so it only fires once routing actually
// resolved to that handler).
func isLongPollRoute(route string) bool {
	return strings.HasSuffix(route, "/connector/tasks/next")
}

// withMiddleware adds request-id, security headers and one structured log
// line per request. The line never includes query strings, headers,
// cookies, Authorization, or any request/response body content (diffs and
// API keys included) — only routing and identity metadata. user_id/agent_id
// are read from a holder that RequireSession/RequireAgent fill in as the
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
		sw := &statusWriter{ResponseWriter: w, status: 200}
		actorLog := &identity.ActorLog{}
		ctx := identity.WithActorLog(r.Context(), actorLog)
		ctx = metrics.WithRouteHolder(ctx)
		r = r.WithContext(ctx)
		start := time.Now()
		withRequestID.ServeHTTP(sw, r)
		ms := time.Since(start).Milliseconds()
		// Not r.Pattern: httpx.WithRequestID (and this closure's own
		// r.WithContext just above) each hand the mux a different
		// *http.Request value than the one held here, so the mux's
		// mutation of r.Pattern is invisible on this r. metrics.HTTPMiddleware
		// wraps the mux directly and records the real route into the
		// context holder attached above instead; see its doc comment.
		route := metrics.RouteFromContext(r.Context())
		if route == "" {
			route = "unmatched"
		}
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
		if actorLog.AgentID != "" {
			fields = append(fields, "agent_id", actorLog.AgentID)
		}
		if sw.status >= 500 {
			log.Error("request failed", fields...)
			return
		}
		// The long-poll route is hit by every connected connector roughly
		// once per fallback interval; at thousands of connectors that is
		// thousands of lines/minute of routine, uninteresting traffic. Only
		// log it when something is actually worth looking at.
		if isLongPollRoute(route) && sw.status < 400 && ms <= 30000 {
			log.Debug("request", fields...)
			return
		}
		log.Info("request", fields...)
	})
}
