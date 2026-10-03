package stacks

import (
	"net/http"
	"strconv"

	"tolerance/internal/platform/httpx"
)

// RegisterPublicRoutes mounts the public stack leaderboards.
func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/stacks", func(w http.ResponseWriter, r *http.Request) {
		days := 0
		if v := r.URL.Query().Get("days"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 3650 {
				httpx.WriteError(w, r, httpx.InvalidBody("days must be a number from 1 to 3650"))
				return
			}
			days = n
		}
		items, err := s.Overall(r.Context(), days)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"days": days, "items": items})
	})
	mux.HandleFunc("GET /api/v1/daily/{day}/stacks", func(w http.ResponseWriter, r *http.Request) {
		day := r.PathValue("day")
		items, err := s.ForDay(r.Context(), day)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"day": day, "items": items})
	})
}
