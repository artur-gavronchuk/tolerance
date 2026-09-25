package identity

import (
	"context"
	"net/http"
	"time"

	"tolerance/internal/platform/captcha"
	"tolerance/internal/platform/clientip"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/metrics"
	"tolerance/internal/platform/ratelimit"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// signupCredentials is credentials plus the optional Cloudflare Turnstile
// token the frontend sends on signup only — never on login. Kept as its
// own type (rather than adding the field to credentials) so login keeps
// rejecting it via httpx.Decode's DisallowUnknownFields.
type signupCredentials struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	TurnstileToken string `json:"turnstile_token"`
}

// clientIP resolves the address to rate-limit on. See clientip.FromRequest
// for what trustProxy means; behind Caddy every request otherwise arrives
// from Caddy's own address, so without it the whole site would share one
// rate-limit bucket.
func clientIP(r *http.Request, trustProxy bool) string {
	return clientip.FromRequest(r, trustProxy)
}

// RegisterAuthRoutes mounts signup, login and logout. They are unauthenticated
// and rate limited per email and per IP. trustProxy must only be true when a
// reverse proxy in front sets X-Real-IP itself (never trust it otherwise).
// captchaVerifier, when non-nil, gates signup on a valid Turnstile token
// (see internal/platform/captcha); nil leaves signup's behavior unchanged
// (any turnstile_token field is accepted and ignored), which is what every
// dev setup and existing test gets since it is only ever non-nil when
// ARENA_TURNSTILE_SECRET is configured.
func RegisterAuthRoutes(mux *http.ServeMux, s *Service, limiter *ratelimit.Limiter, secure, trustProxy bool, captchaVerifier captcha.Verifier) {
	handle := func(fn func(ctx *http.Request, in credentials) (User, string, error), status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			raw, err := httpx.ReadBody(w, r)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			var in credentials
			if err := httpx.Decode(raw, &in); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			email := NormalizeEmail(in.Email)
			if !limiter.Allow("ip:"+clientIP(r, trustProxy), 10, time.Minute) || !limiter.Allow("email:"+email, 10, time.Minute) {
				httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many attempts, try again in a minute"))
				return
			}
			u, token, err := fn(r, in)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			SetSessionCookie(w, token, secure)
			httpx.Respond(w, status, map[string]any{"user": u})
		}
	}
	mux.HandleFunc("POST /api/v1/auth/signup", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in signupCredentials
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		email := NormalizeEmail(in.Email)
		if !limiter.Allow("ip:"+clientIP(r, trustProxy), 10, time.Minute) || !limiter.Allow("email:"+email, 10, time.Minute) {
			httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many attempts, try again in a minute"))
			return
		}
		if captchaVerifier != nil {
			if err := captchaVerifier.Verify(r.Context(), in.TurnstileToken, clientIP(r, trustProxy)); err != nil {
				metrics.CaptchaFailure()
				httpx.WriteError(w, r, httpx.New(http.StatusForbidden, "captcha_failed", "Captcha verification failed, please try again"))
				return
			}
		}
		u, token, err := s.Signup(r.Context(), in.Email, in.Password)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		SetSessionCookie(w, token, secure)
		httpx.Respond(w, http.StatusCreated, map[string]any{"user": u})
	})
	mux.HandleFunc("POST /api/v1/auth/login", handle(func(r *http.Request, in credentials) (User, string, error) {
		return s.Login(r.Context(), in.Email, in.Password)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(SessionCookie); err == nil {
			if err := s.Logout(r.Context(), c.Value); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
		}
		ClearSessionCookie(w, secure)
		w.WriteHeader(http.StatusNoContent)
	})
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
