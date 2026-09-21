package submissions

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idempotency"
)

func listOptions(r *http.Request) (ListOptions, error) {
	o := ListOptions{Kind: r.URL.Query().Get("kind")}
	switch r.URL.Query().Get("include") {
	case "":
	case "pending":
		o.IncludeUnscored = true
	default:
		return o, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "include must be pending", "include", "invalid")
	}
	return o, nil
}

func respondList(w http.ResponseWriter, r *http.Request, items []View, err error) {
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
}

// RegisterPublicRoutes wires the public reads (no authentication).
func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/submissions/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, v)
	})
	mux.HandleFunc("GET /api/v1/competitions/{slug}/submissions", func(w http.ResponseWriter, r *http.Request) {
		o, err := listOptions(r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items, err := s.ListForCompetition(r.Context(), r.PathValue("slug"), o)
		respondList(w, r, items, err)
	})
	mux.HandleFunc("GET /api/v1/agents/{name}/submissions", func(w http.ResponseWriter, r *http.Request) {
		o, err := listOptions(r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items, err := s.ListForAgent(r.Context(), r.PathValue("name"), o)
		respondList(w, r, items, err)
	})
}

// RegisterMeRoutes wires the signed-in user's own reads (user JWT).
func RegisterMeRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/me/submissions", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.ListForOwner(r.Context(), identity.MustFromContext(r.Context()).UserID)
		respondList(w, r, items, err)
	})
}

// RegisterAgentRoutes wires the connector's submit call (API key).
func RegisterAgentRoutes(mux *http.ServeMux, pool *db.Pool, s *Service) {
	mux.HandleFunc("POST /api/v1/agent/attempts/{id}/submission", idempotency.Command(pool, "POST /agent/attempts/submission",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var in Input
			if err := httpx.Decode(raw, &in); err != nil {
				return nil, 0, err
			}
			out, err := s.Submit(r.Context(), actor, r.PathValue("id"), in)
			return out, http.StatusCreated, err
		}))
}
