package standings

import (
	"net/http"
	"strconv"

	"tolerance/internal/platform/httpx"
)

func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items, err := s.Leaderboard(r.Context(), limit)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
}
