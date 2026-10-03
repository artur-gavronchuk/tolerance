package daily

import (
	"net/http"

	"tolerance/internal/platform/httpx"
)

// UserFunc returns the signed-in user's id for a public route, or "" for an anonymous request.
type UserFunc func(r *http.Request) string

// RegisterPublicRoutes mounts the public daily routes; my fills the "my" block for signed-in requests.
func RegisterPublicRoutes(mux *http.ServeMux, s *Service, who UserFunc, my MyFunc) {
	mux.HandleFunc("GET /api/v1/daily", func(w http.ResponseWriter, r *http.Request) {
		d, err := s.Get(r.Context(), Today(), who(r), my)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, d)
	})
	mux.HandleFunc("GET /api/v1/daily/{day}", func(w http.ResponseWriter, r *http.Request) {
		d, err := s.Get(r.Context(), r.PathValue("day"), who(r), my)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, d)
	})
	mux.HandleFunc("GET /api/v1/daily/{day}/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.Leaderboard(r.Context(), r.PathValue("day"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/daily/{day}/reveal", func(w http.ResponseWriter, r *http.Request) {
		rv, err := s.Reveal(r.Context(), r.PathValue("day"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, rv)
	})
	mux.HandleFunc("GET /api/v1/daily/{day}/stats", func(w http.ResponseWriter, r *http.Request) {
		st, err := s.Stats(r.Context(), r.PathValue("day"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, st)
	})
	mux.HandleFunc("GET /api/v1/users/{handle}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.ProfileOf(r.Context(), r.PathValue("handle"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
	mux.HandleFunc("GET /api/v1/days", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.Days(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.Overall(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
}
