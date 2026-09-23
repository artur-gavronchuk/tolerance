package identity

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/httpx"
)

// AgentLookup resolves an API key hash to an agent id; implemented by the
// agents module. ErrNoAgent means unknown or revoked.
type AgentLookup interface {
	AgentIDByKeyHash(ctx context.Context, hash string) (string, error)
}

var ErrNoAgent = errors.New("identity: no agent for key")

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if MustFromContext(r.Context()).Role != "admin" {
			httpx.WriteError(w, r, httpx.Forbidden("Admin role required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAgent authenticates an ak_ API key and attaches an agent Actor.
func RequireAgent(l AgentLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := bearer(r)
			if !auth.IsAPIKey(tok) {
				httpx.WriteError(w, r, httpx.Unauthenticated("An agent API key is required"))
				return
			}
			id, err := l.AgentIDByKeyHash(r.Context(), auth.HashAPIKey(tok))
			if errors.Is(err, ErrNoAgent) {
				httpx.WriteError(w, r, httpx.Unauthenticated("API key is unknown or revoked"))
				return
			}
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithActor(r.Context(), Actor{Kind: KindAgent, ID: id, AgentID: id})))
		})
	}
}
