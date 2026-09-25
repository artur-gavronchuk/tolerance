package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"tolerance/internal/identity"
)

type config struct {
	addr             string
	databaseURL      string
	adminEmails      []string
	secureCookies    bool
	devLogin         bool
	allowNonLoopback bool
	workDir          string
	sandbox          string // "docker" | "fake"
	connectorDir     string // prebuilt connector binaries; "" = none
	publicURL        string
	githubID         string
	githubSecret     string
	googleID         string
	googleSecret     string
	matchInterval    time.Duration
	matchConcurrency int
	botImage         string
}

func loadConfig() (config, error) {
	cfg := config{
		addr:             env("ARENA_ADDR", "127.0.0.1:8080"),
		databaseURL:      os.Getenv("ARENA_APP_DATABASE_URL"),
		secureCookies:    os.Getenv("ARENA_SECURE_COOKIES") == "true",
		devLogin:         os.Getenv("ARENA_DEV_LOGIN") == "true",
		allowNonLoopback: os.Getenv("ARENA_ALLOW_NON_LOOPBACK") == "true",
		workDir:          env("ARENA_WORK_DIR", os.TempDir()),
		sandbox:          env("ARENA_SANDBOX", "docker"),
		connectorDir:     os.Getenv("ARENA_CONNECTOR_DIR"),
		publicURL:        os.Getenv("ARENA_PUBLIC_URL"),
		githubID:         os.Getenv("ARENA_GITHUB_CLIENT_ID"),
		githubSecret:     os.Getenv("ARENA_GITHUB_CLIENT_SECRET"),
		googleID:         os.Getenv("ARENA_GOOGLE_CLIENT_ID"),
		googleSecret:     os.Getenv("ARENA_GOOGLE_CLIENT_SECRET"),
		matchInterval:    20 * time.Second,
		matchConcurrency: 1,
		botImage:         env("ARENA_BOT_IMAGE", "arena-bot-runtime:1"),
	}
	if v := os.Getenv("ARENA_MATCH_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return config{}, errors.New("ARENA_MATCH_INTERVAL must be a valid duration")
		}
		cfg.matchInterval = d
	}
	if v := os.Getenv("ARENA_MATCH_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return config{}, errors.New("ARENA_MATCH_CONCURRENCY must be an integer >= 1")
		}
		cfg.matchConcurrency = n
	}
	for _, e := range strings.Split(os.Getenv("ARENA_ADMIN_EMAILS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			cfg.adminEmails = append(cfg.adminEmails, e)
		}
	}
	host, _, err := net.SplitHostPort(cfg.addr)
	if err != nil || net.ParseIP(host) == nil {
		return config{}, errors.New("ARENA_ADDR must be an ip:port")
	}
	if !net.ParseIP(host).IsLoopback() && !cfg.allowNonLoopback {
		return config{}, errors.New("ARENA_ADDR must be a loopback address; a reverse proxy is expected in front (set ARENA_ALLOW_NON_LOOPBACK=true inside a container)")
	}
	if cfg.databaseURL == "" {
		return config{}, errors.New("ARENA_APP_DATABASE_URL is required")
	}
	if cfg.sandbox != "docker" && cfg.sandbox != "fake" {
		return config{}, errors.New("ARENA_SANDBOX must be docker or fake")
	}
	if cfg.devLogin && cfg.secureCookies {
		return config{}, errors.New("ARENA_DEV_LOGIN is for local runs and CI; it cannot be on with ARENA_SECURE_COOKIES=true")
	}
	for _, p := range [][3]string{{"GITHUB", cfg.githubID, cfg.githubSecret}, {"GOOGLE", cfg.googleID, cfg.googleSecret}} {
		if (p[1] == "") != (p[2] == "") {
			return config{}, fmt.Errorf("set both ARENA_%s_CLIENT_ID and ARENA_%s_CLIENT_SECRET, or neither", p[0], p[0])
		}
	}
	if (cfg.githubID != "" || cfg.googleID != "") && !strings.HasPrefix(cfg.publicURL, "http://") && !strings.HasPrefix(cfg.publicURL, "https://") {
		return config{}, errors.New("ARENA_PUBLIC_URL (http:// or https://) is required when a sign-in provider is configured")
	}
	return cfg, nil
}

// providersFromConfig builds the sign-in providers that have both keys set.
func providersFromConfig(cfg config) map[string]identity.Provider {
	ps := map[string]identity.Provider{}
	if cfg.githubID != "" {
		ps["github"] = &identity.GitHub{ClientID: cfg.githubID, ClientSecret: cfg.githubSecret}
	}
	if cfg.googleID != "" {
		ps["google"] = &identity.Google{ClientID: cfg.googleID, ClientSecret: cfg.googleSecret}
	}
	return ps
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
