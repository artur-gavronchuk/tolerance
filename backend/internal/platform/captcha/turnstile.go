// Package captcha verifies Cloudflare Turnstile tokens for the signup
// endpoint. It is deliberately tiny and defined behind an interface so
// handler tests can inject a fake instead of calling out to Cloudflare.
package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrFailed is returned (wrapped, where relevant) whenever a token could
// not be verified, whatever the reason — missing token, Cloudflare saying
// no, or a network/timeout error talking to Cloudflare. Callers only need
// to know verification failed, not why, since the response to the client
// is the same 403 either way.
var ErrFailed = errors.New("captcha verification failed")

const defaultVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// Verifier checks a Turnstile token presented by a signup request. remoteIP
// is the caller's real client address (see clientip.FromRequest), passed
// through to Cloudflare as an extra signal; it may be empty.
type Verifier interface {
	Verify(ctx context.Context, token, remoteIP string) error
}

// Turnstile is the real Verifier, calling Cloudflare's siteverify endpoint.
type Turnstile struct {
	Secret string
	// VerifyURL overrides the Cloudflare endpoint; tests point it at an
	// httptest server. Left empty, defaultVerifyURL is used.
	VerifyURL string
	// Timeout bounds the whole verify call. Left zero, it defaults to 5s.
	Timeout time.Duration
	// Client is the HTTP client used to call VerifyURL. Left nil, a fresh
	// http.Client is used for each verifier (there is only ever one, built
	// once at startup).
	Client *http.Client
}

// NewTurnstile builds a Verifier against the real Cloudflare endpoint with
// the package defaults (5s timeout).
func NewTurnstile(secret string) *Turnstile {
	return &Turnstile{Secret: secret}
}

type siteverifyResponse struct {
	Success bool `json:"success"`
}

// Verify posts token to Cloudflare's siteverify endpoint and reports
// whether it was accepted. A missing token, a non-success verdict, and a
// network error or timeout are all reported as ErrFailed (or a wrapped
// form of it) — the caller does not need to distinguish them, only log
// them if it wants detail.
func (t *Turnstile) Verify(ctx context.Context, token, remoteIP string) error {
	if strings.TrimSpace(token) == "" {
		return ErrFailed
	}

	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	verifyURL := t.VerifyURL
	if verifyURL == "" {
		verifyURL = defaultVerifyURL
	}

	form := url.Values{}
	form.Set("secret", t.Secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.Join(ErrFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Join(ErrFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrFailed
	}

	var out siteverifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return errors.Join(ErrFailed, err)
	}
	if !out.Success {
		return ErrFailed
	}
	return nil
}
