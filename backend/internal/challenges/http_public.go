package challenges

import (
	"net/http"

	"tolerance/internal/platform/httpx"
)

// RegisterPublicRoutes mounts the challenge index and one challenge's page. Both
// are unauthenticated: a competition nobody can look at is not a competition.
func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/challenges", func(w http.ResponseWriter, r *http.Request) {
		lists, err := s.List(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, lists)
	})

	mux.HandleFunc("GET /api/v1/challenges/{slug}", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Public(r.Context(), r.PathValue("slug"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, v)
	})
}
