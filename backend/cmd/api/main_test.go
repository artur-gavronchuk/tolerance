package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/ratelimit"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	d := dbtest.New(t)
	cfg := config{addr: "127.0.0.1:0", adminEmails: []string{"admin@arena.local"}, sandbox: "fake"}
	dp := deps{pool: d.AppPool, log: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		users: identity.NewService(d.AppPool, cfg.adminEmails), agents: agents.NewService(d.AppPool), limiter: ratelimit.New(nil)}
	srv := httptest.NewServer(newHandler(cfg, dp))
	t.Cleanup(srv.Close)
	return srv
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
