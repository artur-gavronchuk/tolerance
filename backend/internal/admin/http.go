package admin

import (
	"net/http"

	"tolerance/internal/platform/httpx"
)

// RegisterRoutes mounts the admin routes. The caller wraps them in the session and admin guards.
func RegisterRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/admin/pulse", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Pulse(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
	mux.HandleFunc("GET /api/v1/admin/recent", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.Recent(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
}
