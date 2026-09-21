package main

import (
	"errors"
	"net"
	"os"
	"strings"
)

type config struct {
	addr         string
	webOrigin    string
	databaseURL  string
	oidcIssuer   string
	oidcAudience string
	oidcJWKSURL  string
	adminEmails  []string

	// publicWebURL is the frontend origin used to build result links.
	publicWebURL string
	// allowLoopbackPreview accepts http://127.0.0.1 and http://localhost as
	// preview URLs. Local development only: it lets the checker open apps
	// served from the developer's own machine.
	allowLoopbackPreview bool
}

func loadConfig() (config, error) {
	cfg := config{
		addr:         env("ARENA_ADDR", "127.0.0.1:8080"),
		webOrigin:    os.Getenv("ARENA_WEB_ORIGIN"),
		databaseURL:  os.Getenv("ARENA_APP_DATABASE_URL"),
		oidcIssuer:   os.Getenv("ARENA_OIDC_ISSUER"),
		oidcAudience: env("ARENA_OIDC_AUDIENCE", "arena-web"),
		oidcJWKSURL:  os.Getenv("ARENA_OIDC_JWKS_URL"),

		publicWebURL:         env("ARENA_PUBLIC_WEB_URL", "http://localhost:3000"),
		allowLoopbackPreview: os.Getenv("ARENA_ALLOW_LOOPBACK_PREVIEW") == "true",
	}
	for _, e := range strings.Split(os.Getenv("ARENA_ADMIN_EMAILS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			cfg.adminEmails = append(cfg.adminEmails, e)
		}
	}
	host, _, err := net.SplitHostPort(cfg.addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return config{}, errors.New("ARENA_ADDR must be a loopback address; a reverse proxy is expected in front")
	}
	if cfg.databaseURL == "" {
		return config{}, errors.New("ARENA_APP_DATABASE_URL is required")
	}
	if cfg.oidcIssuer == "" || cfg.oidcJWKSURL == "" {
		return config{}, errors.New("ARENA_OIDC_ISSUER and ARENA_OIDC_JWKS_URL are required")
	}
	if cfg.webOrigin == "" {
		return config{}, errors.New("ARENA_WEB_ORIGIN is required so CORS allows exactly one origin")
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
