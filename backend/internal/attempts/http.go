package attempts

import (
	"net/http"

	"tolerance/internal/competitions"
	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

type startInput struct {
	Kind        string      `json:"kind"`
	AgentConfig AgentConfig `json:"agent_config"`
}

type eventsInput struct {
	Events []EventInput `json:"events"`
}

type voidInput struct {
	Reason string `json:"reason"`
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	raw, err := httpx.ReadBody(w, r)
	if err == nil {
		err = httpx.Decode(raw, dst)
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return false
	}
	return true
}

// RegisterAgentRoutes wires the connector's routes (API key). Reading the
// task needs the competition, so it takes the competitions service.
func RegisterAgentRoutes(mux *http.ServeMux, s *Service, comps *competitions.Service) {
	mux.HandleFunc("GET /api/v1/agent/competitions/{slug}/task", func(w http.ResponseWriter, r *http.Request) {
		actor := identity.MustFromContext(r.Context())
		c, err := comps.GetPublicBySlug(r.Context(), r.PathValue("slug"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		list, err := s.ListForAgent(r.Context(), actor.AgentID, c.ID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		available, err := s.OfficialAvailable(r.Context(), actor.AgentID, c.ID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		datasetURL := ""
		if c.CheckSuite != "" {
			datasetURL = "/api/v1/competitions/" + c.Slug + "/dataset"
		}
		httpx.Respond(w, http.StatusOK, map[string]any{
			"competition":        c.Public(),
			"dataset_url":        datasetURL,
			"attempts":           list,
			"official_available": available && c.Status == competitions.StatusActive,
		})
	})

	mux.HandleFunc("POST /api/v1/agent/competitions/{slug}/attempts", func(w http.ResponseWriter, r *http.Request) {
		var in startInput
		if !decode(w, r, &in) {
			return
		}
		a, resumed, err := s.Start(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("slug"), in.Kind, in.AgentConfig)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		status := http.StatusCreated
		if resumed {
			status = http.StatusOK
		}
		httpx.Respond(w, status, map[string]any{"attempt": a, "resumed": resumed})
	})

	mux.HandleFunc("POST /api/v1/agent/attempts/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		var in eventsInput
		if !decode(w, r, &in) {
			return
		}
		if err := s.AddEvents(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id"), in.Events); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v1/agent/attempts/{id}/abandon", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Abandon(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id")); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// RegisterAdminRoutes wires the admin routes (user JWT, admin role).
func RegisterAdminRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/admin/attempts/{id}/void", func(w http.ResponseWriter, r *http.Request) {
		var in voidInput
		if !decode(w, r, &in) {
			return
		}
		if err := s.Void(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id"), in.Reason); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
