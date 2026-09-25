package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"tolerance/internal/platform/clientip"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/metrics"
	"tolerance/internal/platform/ratelimit"
)

// clientIP resolves the address to rate-limit on. See clientip.FromRequest
// for what trustProxy means; behind Caddy every request otherwise arrives
// from Caddy's own address, so without it the whole site would share one
// rate-limit bucket.
func clientIP(r *http.Request, trustProxy bool) string {
	return clientip.FromRequest(r, trustProxy)
}

// AuthConfig is what the auth routes need from the server config.
type AuthConfig struct {
	Providers map[string]Provider // "github", "google": only the configured ones
	PublicURL string              // base of redirect_uri, e.g. https://tolerance.cc
	DevLogin  bool                // mounts POST /auth/dev; never on in production
	Secure    bool                // Secure flag on cookies
	// TrustProxy must only be true when a reverse proxy in front sets
	// X-Real-IP itself (never trust it otherwise) — see clientip.FromRequest.
	TrustProxy bool
}

// OAuthCookie carries the state and PKCE verifier between start and callback.
const OAuthCookie = "arena_oauth"

// RegisterAuthRoutes mounts the unauthenticated auth routes: the provider
// list, logout, OAuth start/callback, and (when enabled) the development
// sign-in.
func RegisterAuthRoutes(mux *http.ServeMux, s *Service, limiter *ratelimit.Limiter, cfg AuthConfig) {
	mux.HandleFunc("GET /api/v1/auth/providers", func(w http.ResponseWriter, r *http.Request) {
		names := slices.Sorted(maps.Keys(cfg.Providers))
		if names == nil {
			names = []string{}
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"providers": names, "dev_login": cfg.DevLogin})
	})
	if cfg.DevLogin {
		mux.HandleFunc("POST /api/v1/auth/dev", devSignIn(s, limiter, cfg.Secure, cfg.TrustProxy))
	}
	mux.HandleFunc("GET /api/v1/auth/{provider}/start", oauthStart(limiter, cfg))
	mux.HandleFunc("GET /api/v1/auth/{provider}/callback", oauthCallback(s, limiter, cfg))
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(SessionCookie); err == nil {
			if err := s.Logout(r.Context(), c.Value); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
		}
		ClearSessionCookie(w, cfg.Secure)
		w.WriteHeader(http.StatusNoContent)
	})
}

func (c AuthConfig) redirectURL(provider string) string {
	return strings.TrimRight(c.PublicURL, "/") + "/api/v1/auth/" + provider + "/callback"
}

func setOAuthCookie(w http.ResponseWriter, value string, maxAge int, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: OAuthCookie, Value: value, Path: "/api/v1/auth/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

// oauthStart sends the browser to the provider. state and the PKCE verifier
// stay in a ten-minute cookie; the callback checks them.
func oauthStart(limiter *ratelimit.Limiter, cfg AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("provider")
		p, ok := cfg.Providers[name]
		if !ok {
			httpx.WriteError(w, r, httpx.NotFound())
			return
		}
		if !limiter.Allow("oauth:ip:"+clientIP(r, cfg.TrustProxy), 20, time.Minute) {
			metrics.RateLimited("oauth_start")
			http.Redirect(w, r, "/login?error=rate_limited", http.StatusFound)
			return
		}
		state, _, err := newSessionToken() // 32 random bytes, hex
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		st := oauthState{Provider: name, State: state, Verifier: oauth2.GenerateVerifier(), Next: safeNext(r.URL.Query().Get("next"))}
		setOAuthCookie(w, encodeState(st), 600, cfg.Secure)
		http.Redirect(w, r, p.AuthCodeURL(st.State, st.Verifier, cfg.redirectURL(name)), http.StatusFound)
	}
}

// oauthCallback finishes the sign-in. It is a browser navigation, so every
// failure is a redirect to /login with a code the page explains.
func oauthCallback(s *Service, limiter *ratelimit.Limiter, cfg AuthConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("provider")
		p, ok := cfg.Providers[name]
		if !ok {
			httpx.WriteError(w, r, httpx.NotFound())
			return
		}
		setOAuthCookie(w, "", -1, cfg.Secure) // single use, whatever happens next
		fail := func(code string) {
			metrics.OAuthLogin(name, code)
			http.Redirect(w, r, "/login?error="+code, http.StatusFound)
		}
		if !limiter.Allow("oauth:ip:"+clientIP(r, cfg.TrustProxy), 20, time.Minute) {
			metrics.RateLimited("oauth_callback")
			fail("rate_limited")
			return
		}
		q := r.URL.Query()
		if q.Get("error") != "" {
			fail("oauth_denied")
			return
		}
		c, err := r.Cookie(OAuthCookie)
		if err != nil {
			fail("oauth_state")
			return
		}
		st, err := decodeState(c.Value)
		if err != nil || st.Provider != name || subtle.ConstantTimeCompare([]byte(st.State), []byte(q.Get("state"))) != 1 {
			fail("oauth_state")
			return
		}
		id, err := p.Identify(r.Context(), q.Get("code"), st.Verifier, cfg.redirectURL(name))
		if err != nil {
			slog.WarnContext(r.Context(), "oauth identify failed", "provider", name, "err", err)
			fail("oauth_failed")
			return
		}
		id.Provider = name
		_, token, err := s.SignIn(r.Context(), id)
		if errors.Is(err, ErrEmailUnverified) {
			fail("email_unverified")
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "oauth sign-in failed", "provider", name, "err", err)
			fail("oauth_failed")
			return
		}
		metrics.OAuthLogin(name, "ok")
		SetSessionCookie(w, token, cfg.Secure)
		// st.Next was already validated by safeNext when the cookie was set,
		// but the cookie is not signed, so re-check before trusting it here.
		http.Redirect(w, r, safeNext(st.Next), http.StatusFound)
	}
}

// devSignIn signs in as any email, no password: local runs and CI only.
func devSignIn(s *Service, limiter *ratelimit.Limiter, secure, trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in struct {
			Email string `json:"email"`
		}
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		email := NormalizeEmail(in.Email)
		// mail.ParseAddress also accepts "Name <a@b.c>"; require the whole
		// field to be a bare address, or a display name would sign in as a
		// mangled email that never matches what the person typed.
		addr, err := mail.ParseAddress(email)
		if err != nil || addr.Address != email || len(email) > 254 {
			httpx.WriteError(w, r, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "email must be a valid address", "email", "invalid"))
			return
		}
		if !limiter.Allow("ip:"+clientIP(r, trustProxy), 10, time.Minute) || !limiter.Allow("email:"+email, 10, time.Minute) {
			httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many attempts, try again in a minute"))
			return
		}
		u, token, err := s.SignIn(r.Context(), Identity{Provider: "dev", Subject: email, Email: email, EmailVerified: true})
		if errors.Is(err, ErrEmailUnverified) {
			httpx.WriteError(w, r, httpx.New(http.StatusForbidden, "email_unverified", "Your account has no verified email address"))
			return
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		SetSessionCookie(w, token, secure)
		httpx.Respond(w, http.StatusOK, map[string]any{"user": u})
	}
}

// RegisterMeRoute mounts GET /me. The agent part comes from the agents
// module as an opaque value so identity does not import it.
func RegisterMeRoute(mux *http.ServeMux, s *Service, agentFor func(ctx context.Context, userID string) (any, error)) {
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		actor := MustFromContext(r.Context())
		u, err := s.Get(r.Context(), actor.UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		agent, err := agentFor(r.Context(), actor.UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"user": u, "agent": agent})
	})
}
