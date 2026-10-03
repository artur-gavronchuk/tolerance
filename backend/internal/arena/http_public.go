package arena

import (
	"net/http"
	"strconv"

	"tolerance/internal/platform/httpx"
)

// parseLimit reads the "limit" query parameter: missing means def, a value over
// max is clamped down to max, and anything that doesn't parse as a positive
// integer is a 422 validation_failed on fields.limit. Same contract as the
// tanks leaderboard's, kept local because the two are separate domains.
func parseLimit(r *http.Request, def, max int) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed", "limit must be a positive integer", "limit", "invalid")
	}
	if n > max {
		n = max
	}
	return n, nil
}

// RegisterPublicRoutes mounts the arena's public, unauthenticated reads.
func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/arena/skills", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.Skills(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})

	mux.HandleFunc("GET /api/v1/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		skill := r.URL.Query().Get("skill")
		if skill == "" {
			httpx.WriteError(w, r, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed",
				"skill is required", "skill", "required"))
			return
		}
		limit, err := parseLimit(r, 100, 500)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items, err := s.Leaderboard(r.Context(), skill, limit)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
}
