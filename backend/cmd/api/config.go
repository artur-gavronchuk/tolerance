package main

import (
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type config struct {
	addr             string
	databaseURL      string
	adminEmails      []string
	secureCookies    bool
	allowNonLoopback bool
	workDir          string
	sandbox          string // "docker" | "fake"
	connectorDir     string // prebuilt connector binaries; "" = none
	matchInterval    time.Duration
	matchConcurrency int
	botImage         string
}

func loadConfig() (config, error) {
	cfg := config{
		addr:             env("ARENA_ADDR", "127.0.0.1:8080"),
		databaseURL:      os.Getenv("ARENA_APP_DATABASE_URL"),
		secureCookies:    os.Getenv("ARENA_SECURE_COOKIES") == "true",
		allowNonLoopback: os.Getenv("ARENA_ALLOW_NON_LOOPBACK") == "true",
		workDir:          env("ARENA_WORK_DIR", os.TempDir()),
		sandbox:          env("ARENA_SANDBOX", "docker"),
		connectorDir:     os.Getenv("ARENA_CONNECTOR_DIR"),
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
	return cfg, nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
