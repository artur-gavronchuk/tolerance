// Command api runs the slice 1 HTTP service: identity, campaigns, missions
// and budget over a single PostgreSQL database. Execution (cmd/runner) and
// independent evaluation (cmd/evaluator) are separate binaries added in
// later slices; this process never runs untrusted code.
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

	"tolerance/internal/budget"
	"tolerance/internal/campaigns"
	"tolerance/internal/identity"
	"tolerance/internal/missions"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/db"
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

	pool, err := db.Open(ctx, cfg.databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer pool.Close()

	verifier := auth.NewVerifier(cfg.oidcIssuer, cfg.oidcAudience, cfg.oidcJWKSURL, 10*time.Minute)
	identityService := identity.NewService(pool)
	campaignService := campaigns.NewService(pool)
	missionService := missions.NewService(pool, campaignService)
	budgetService := budget.NewService(pool)

	handler := newHandler(cfg, pool, verifier, identityService, campaignService, missionService, budgetService)

	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           handler,
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

	log.Printf("FORGE api listening on http://%s", cfg.addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func newHandler(cfg config, pool *db.Pool, verifier *auth.Verifier, identityService *identity.Service,
	campaignService *campaigns.Service, missionService *missions.Service, budgetService *budget.Service) http.Handler {

	// orgRoutes holds every organization-scoped route: campaigns, missions,
	// budget, and (from slice 2 onward) everything else. It sits behind
	// RequireOrganization so a missing X-Organization-Id fails clearly
	// instead of silently touching the wrong (empty) scope.
	orgRoutes := http.NewServeMux()
	campaigns.RegisterRoutes(orgRoutes, pool, campaignService)
	missions.RegisterRoutes(orgRoutes, pool, missionService)
	budget.RegisterRoutes(orgRoutes, pool, budgetService)

	api := http.NewServeMux()
	identity.RegisterRoutes(api, identityService) // GET /me, POST /organizations: need no organization
	api.Handle("/", identity.RequireOrganization(orgRoutes))

	authenticated := identity.Middleware(verifier, identityService)(api)

	top := http.NewServeMux()
	top.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	top.Handle("/api/", authenticated)

	return withMiddleware(top, cfg)
}

func withMiddleware(next http.Handler, cfg config) http.Handler {
	withRequestID := httpx.WithRequestID(func() string { return idgen.New("req") })(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if !applyCORS(w, r, cfg.spaOrigin) {
			return
		}
		withRequestID.ServeHTTP(w, r)
	})
}

// applyCORS allows only the configured SPA origin, which runs on its own
// domain per the frontend design (no shared cookies, no wildcard). It
// returns false after fully answering an OPTIONS preflight, telling the
// caller not to continue down the handler chain.
func applyCORS(w http.ResponseWriter, r *http.Request, allowedOrigin string) bool {
	origin := r.Header.Get("Origin")
	if origin != "" && origin == allowedOrigin {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Organization-Id")
		w.Header().Set("Access-Control-Max-Age", "600")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}

type config struct {
	addr         string
	databaseURL  string
	oidcIssuer   string
	oidcAudience string
	oidcJWKSURL  string
	spaOrigin    string
}

func loadConfig() (config, error) {
	cfg := config{
		addr:         env("FORGE_ADDR", "127.0.0.1:8080"),
		databaseURL:  os.Getenv("FORGE_APP_DATABASE_URL"),
		oidcIssuer:   os.Getenv("FORGE_OIDC_ISSUER"),
		oidcAudience: env("FORGE_OIDC_AUDIENCE", "forge-api"),
		oidcJWKSURL:  os.Getenv("FORGE_OIDC_JWKS_URL"),
		spaOrigin:    os.Getenv("FORGE_SPA_ORIGIN"),
	}
	host, _, err := net.SplitHostPort(cfg.addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return config{}, errors.New("FORGE_ADDR must be a loopback address in this slice; a reverse proxy is expected in front of it")
	}
	if cfg.databaseURL == "" {
		return config{}, errors.New("FORGE_APP_DATABASE_URL is required")
	}
	if cfg.oidcIssuer == "" || cfg.oidcJWKSURL == "" {
		return config{}, errors.New("FORGE_OIDC_ISSUER and FORGE_OIDC_JWKS_URL are required")
	}
	if cfg.spaOrigin == "" {
		return config{}, errors.New("FORGE_SPA_ORIGIN is required so CORS has exactly one origin to allow")
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
