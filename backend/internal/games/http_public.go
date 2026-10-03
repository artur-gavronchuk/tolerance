package games

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"tolerance/internal/games/tanks"
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

// RegisterAdminRoutes registers what only an admin (or a local dev login) may do: starting a tournament now.
// The caller wraps the mux in the session and role checks.
func RegisterAdminRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/tanks/tournaments", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Size int `json:"size"`
		}
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if len(raw) > 0 {
			if err := httpx.Decode(raw, &in); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
		}
		t, err := s.StartTournament(r.Context(), in.Size)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, t)
	})
}

// RegisterPublicRoutes registers the tanks arena's public, unauthenticated routes: the season ladder, the
// match feed and single matches (with replays), bot profiles, and the live broadcast schedule.
func RegisterPublicRoutes(mux *http.ServeMux, s *Service, who func(*http.Request) string) {
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

	mux.HandleFunc("GET /api/v1/tanks/starter/{kit}", func(w http.ResponseWriter, r *http.Request) {
		lang, ok := strings.CutSuffix(r.PathValue("kit"), ".zip")
		if !ok {
			httpx.WriteError(w, r, httpx.NotFound())
			return
		}
		data, name, err := tanks.StarterZip(lang)
		if err != nil {
			httpx.WriteError(w, r, httpx.NotFound())
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
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

	mux.HandleFunc("GET /api/v1/tanks/showcase", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.Showcase(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, out)
	})

	mux.HandleFunc("GET /api/v1/tanks/seasons", func(w http.ResponseWriter, r *http.Request) {
		limit, err := parseLimit(r, 24, 100)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items, err := s.Seasons(r.Context(), limit)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})

	// {id} is a season id like "2026-10", or "current".
	mux.HandleFunc("GET /api/v1/tanks/seasons/{id}", func(w http.ResponseWriter, r *http.Request) {
		offset, limit, err := httpx.PageParams(r, 100, 500)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		d, err := s.Season(r.Context(), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		// Total keeps the real size; You is the signed-in viewer's best-ranked bot when it is outside the slice.
		if uid := who(r); uid != "" {
			for _, e := range d.Standings {
				if e.ownerID == uid {
					e := e
					d.You = &e
					break
				}
			}
		}
		d.Standings = d.Standings[min(offset, len(d.Standings)):min(offset+limit, len(d.Standings))]
		if d.You != nil && d.You.Rank > offset && d.You.Rank <= offset+limit {
			d.You = nil
		}
		httpx.Respond(w, http.StatusOK, d)
	})

	mux.HandleFunc("GET /api/v1/tanks/tournaments", func(w http.ResponseWriter, r *http.Request) {
		limit, err := parseLimit(r, 30, 100)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items, err := s.Tournaments(r.Context(), r.URL.Query().Get("status"), limit)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items, "now": time.Now().UTC()})
	})

	mux.HandleFunc("GET /api/v1/tanks/tournaments/{id}", func(w http.ResponseWriter, r *http.Request) {
		t, err := s.Tournament(r.Context(), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, t)
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
