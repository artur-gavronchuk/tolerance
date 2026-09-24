package identity

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/ratelimit"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

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

// RegisterAuthRoutes mounts signup, login and logout. They are unauthenticated
// and rate limited per email and per IP.
func RegisterAuthRoutes(mux *http.ServeMux, s *Service, limiter *ratelimit.Limiter, secure bool) {
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
			if !limiter.Allow("ip:"+clientIP(r), 10, time.Minute) || !limiter.Allow("email:"+email, 10, time.Minute) {
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
	mux.HandleFunc("POST /api/v1/auth/signup", handle(func(r *http.Request, in credentials) (User, string, error) {
		return s.Signup(r.Context(), in.Email, in.Password)
	}, http.StatusCreated))
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
