package identity

import (
	"context"
	"net/http"

	"tolerance/internal/platform/httpx"
)

// RegisterRoutes mounts /me. meAgent returns the caller's private agent
// view (or nil) — supplied by the agents module so identity does not
// import it.
func RegisterRoutes(mux *http.ServeMux, s *Service, meAgent func(ctx context.Context, userID string) (any, error)) {
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		actor := MustFromContext(r.Context())
		u, err := s.Get(r.Context(), actor.UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		agent, err := meAgent(r.Context(), actor.UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"user": u, "agent": agent})
	})
	mux.HandleFunc("PATCH /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		actor := MustFromContext(r.Context())
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in UpdateInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		u, err := s.Update(r.Context(), actor.UserID, in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, u)
	})
}
