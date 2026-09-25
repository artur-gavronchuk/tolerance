package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/captcha"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/proofs"
)

// fakeCaptchaVerifier lets the handler-level test control Turnstile
// verification without calling out to Cloudflare: it succeeds only for the
// configured token.
type fakeCaptchaVerifier struct{ goodToken string }

func (f fakeCaptchaVerifier) Verify(_ context.Context, token, _ string) error {
	if token == f.goodToken {
		return nil
	}
	return captcha.ErrFailed
}

// newCaptchaTestServer builds a minimal handler-level test server, mirroring
// newE2E's setup, with the given captcha verifier wired into deps exactly
// as main.go wires the real one from ARENA_TURNSTILE_SECRET.
func newCaptchaTestServer(t *testing.T, verifier captcha.Verifier) *httptest.Server {
	t.Helper()
	d := dbtest.New(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg := config{addr: "127.0.0.1:0", sandbox: "fake"}
	scale := scaleConfig{role: "all", hashConcurrency: 4, workerConcurrency: 1,
		rateIPRPS: 1e6, rateIPBurst: 1_000_000, rateKeyRPS: 1e6, rateKeyBurst: 1_000_000}
	ps := proofs.NewService(d.AppPool)
	dp := deps{
		pool: d.AppPool, log: log, limiter: ratelimit.New(nil), users: identity.NewService(d.AppPool, cfg.adminEmails),
		agents: agents.NewService(d.AppPool, ps), proofs: ps,
		ipLimiter:       ratelimit.NewTokenBuckets(scale.rateIPRPS, scale.rateIPBurst, 100),
		keyLimiter:      ratelimit.NewTokenBuckets(scale.rateKeyRPS, scale.rateKeyBurst, 100),
		longPoll:        ratelimit.NewConcurrencyLimiter(2),
		captchaVerifier: verifier,
	}
	srv := httptest.NewServer(newHandler(cfg, scale, dp))
	t.Cleanup(srv.Close)
	return srv
}

func postSignup(t *testing.T, srv *httptest.Server, body map[string]string) (int, httpx.Problem) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	resp, err := http.Post(srv.URL+"/api/v1/auth/signup", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("post signup: %v", err)
	}
	defer resp.Body.Close()
	var problem httpx.Problem
	_ = json.NewDecoder(resp.Body).Decode(&problem)
	return resp.StatusCode, problem
}

func TestSignup_CaptchaConfigured_RejectsMissingOrBadToken(t *testing.T) {
	srv := newCaptchaTestServer(t, fakeCaptchaVerifier{goodToken: "good-token"})

	code, problem := postSignup(t, srv, map[string]string{"email": "no-token@example.com", "password": "longenough1"})
	if code != http.StatusForbidden || problem.Code != "captcha_failed" {
		t.Fatalf("missing token: status %d code %q, want 403 captcha_failed", code, problem.Code)
	}

	code, problem = postSignup(t, srv, map[string]string{"email": "bad-token@example.com", "password": "longenough1", "turnstile_token": "wrong"})
	if code != http.StatusForbidden || problem.Code != "captcha_failed" {
		t.Fatalf("bad token: status %d code %q, want 403 captcha_failed", code, problem.Code)
	}
}

func TestSignup_CaptchaConfigured_AcceptsGoodToken(t *testing.T) {
	srv := newCaptchaTestServer(t, fakeCaptchaVerifier{goodToken: "good-token"})

	code, problem := postSignup(t, srv, map[string]string{"email": "good-token@example.com", "password": "longenough1", "turnstile_token": "good-token"})
	if code != http.StatusCreated {
		t.Fatalf("good token: status %d code %q, want 201", code, problem.Code)
	}
}

func TestSignup_NoCaptchaConfigured_TokenFieldAcceptedAndIgnored(t *testing.T) {
	srv := newCaptchaTestServer(t, nil)

	code, problem := postSignup(t, srv, map[string]string{"email": "no-captcha@example.com", "password": "longenough1", "turnstile_token": "anything-or-empty"})
	if code != http.StatusCreated {
		t.Fatalf("no captcha configured: status %d code %q, want 201", code, problem.Code)
	}

	// Without the field at all, signup must still work (existing behavior).
	code, problem = postSignup(t, srv, map[string]string{"email": "no-captcha-no-field@example.com", "password": "longenough1"})
	if code != http.StatusCreated {
		t.Fatalf("no captcha configured, no field: status %d code %q, want 201", code, problem.Code)
	}
}
