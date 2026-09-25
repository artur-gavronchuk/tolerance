package identity

import (
	"context"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/ratelimit"
)

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// AuthConfig is what the auth routes need from the server config.
type AuthConfig struct {
	DevLogin bool // mounts POST /auth/dev; never on in production
	Secure   bool // Secure flag on cookies
}

// RegisterAuthRoutes mounts the unauthenticated auth routes: the provider
// list, logout, and (when enabled) the development sign-in.
func RegisterAuthRoutes(mux *http.ServeMux, s *Service, limiter *ratelimit.Limiter, cfg AuthConfig) {
	mux.HandleFunc("GET /api/v1/auth/providers", func(w http.ResponseWriter, r *http.Request) {
		httpx.Respond(w, http.StatusOK, map[string]any{"providers": []string{}, "dev_login": cfg.DevLogin})
	})
	if cfg.DevLogin {
		mux.HandleFunc("POST /api/v1/auth/dev", devSignIn(s, limiter, cfg.Secure))
	}
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

// devSignIn signs in as any email, no password: local runs and CI only.
func devSignIn(s *Service, limiter *ratelimit.Limiter, secure bool) http.HandlerFunc {
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
		if _, err := mail.ParseAddress(email); err != nil || len(email) > 254 {
			httpx.WriteError(w, r, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "email must be a valid address", "email", "invalid"))
			return
		}
		if !limiter.Allow("ip:"+clientIP(r), 10, time.Minute) || !limiter.Allow("email:"+email, 10, time.Minute) {
			httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many attempts, try again in a minute"))
			return
		}
		u, token, err := s.SignIn(r.Context(), Identity{Provider: "dev", Subject: email, Email: email, EmailVerified: true})
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
