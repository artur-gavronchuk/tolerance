package games

import (
	"net/http"
	"strconv"

	"tolerance/internal/platform/httpx"
)

// parseLimit reads the "limit" query parameter: missing means def, a value over max is clamped down to
// max, and anything that doesn't parse as a positive integer is a 422 validation_failed on fields.limit.
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

// RegisterPublicRoutes registers the tanks arena's public, unauthenticated routes: the leaderboard, the
// match feed and single matches (with replays), bot profiles, and the live broadcast schedule.
func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/tanks/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		limit, err := parseLimit(r, 100, 500)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items, err := s.Leaderboard(r.Context(), limit)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})

	mux.HandleFunc("GET /api/v1/tanks/matches", func(w http.ResponseWriter, r *http.Request) {
		limit, err := parseLimit(r, 20, 50)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items, err := s.Matches(r.Context(), r.URL.Query().Get("bot_id"), limit)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})

	mux.HandleFunc("GET /api/v1/tanks/matches/{id}", func(w http.ResponseWriter, r *http.Request) {
		m, err := s.Match(r.Context(), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, m)
	})

	mux.HandleFunc("GET /api/v1/tanks/matches/{id}/replay", func(w http.ResponseWriter, r *http.Request) {
		data, err := s.Replay(r.Context(), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /api/v1/tanks/bots/{id}", func(w http.ResponseWriter, r *http.Request) {
		b, err := s.Bot(r.Context(), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, b)
	})

	mux.HandleFunc("GET /api/v1/tanks/live", func(w http.ResponseWriter, r *http.Request) {
		live, err := s.Live(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, live)
	})
}
