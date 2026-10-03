package identity

import (
	"errors"
	"net/http"

	"tolerance/internal/platform/httpx"
)

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if MustFromContext(r.Context()).Role != "admin" {
			httpx.WriteError(w, r, httpx.Forbidden("Admin role required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireSession authenticates the arena_session cookie and attaches a user Actor.
func RequireSession(s *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(SessionCookie)
			if err != nil {
				httpx.WriteError(w, r, httpx.Unauthenticated("Sign in required"))
				return
			}
			u, err := s.UserBySession(r.Context(), c.Value)
			if errors.Is(err, ErrNoSession) {
				httpx.WriteError(w, r, httpx.Unauthenticated("Session expired, sign in again"))
				return
			}
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			if h := actorLogFromContext(r.Context()); h != nil {
				h.UserID = u.ID
			}
			next.ServeHTTP(w, r.WithContext(WithActor(r.Context(),
				Actor{Kind: KindUser, ID: u.ID, UserID: u.ID, Role: u.Role})))
		})
	}
}

// OptionalUserID returns a function that resolves the session cookie of a request on a public route to a
// user id, or "" when the request is anonymous or its session is gone.
func OptionalUserID(s *Service) func(r *http.Request) string {
	return func(r *http.Request) string {
		c, err := r.Cookie(SessionCookie)
		if err != nil {
			return ""
		}
		u, err := s.UserBySession(r.Context(), c.Value)
		if err != nil {
			return ""
		}
		return u.ID
	}
}
