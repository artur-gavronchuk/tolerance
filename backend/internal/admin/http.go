package admin

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

type reasonInput struct {
	Reason string `json:"reason"`
}

// reasonOf reads the {"reason": "..."} body every mutating admin route takes.
func reasonOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw, err := httpx.ReadBody(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return "", false
	}
	var in reasonInput
	if err := httpx.Decode(raw, &in); err != nil {
		httpx.WriteError(w, r, err)
		return "", false
	}
	return in.Reason, true
}

// RegisterRoutes mounts the operator API. Every route here must be behind
// identity.RequireAdmin, which in turn must be inside the session middleware —
// it reads the session actor's role.
func RegisterRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/admin/skill-tasks", func(w http.ResponseWriter, r *http.Request) {
		skill := r.URL.Query().Get("skill")
		if skill == "" {
			httpx.WriteError(w, r, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed",
				"skill is required", "skill", "required"))
			return
		}
		items, err := s.TaskStats(r.Context(), skill)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})

	mux.HandleFunc("POST /api/v1/admin/skill-tasks/{slug}/retire", func(w http.ResponseWriter, r *http.Request) {
		reason, ok := reasonOf(w, r)
		if !ok {
			return
		}
		if err := s.RetireTask(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("slug"), reason); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"status": "retired"})
	})

	mux.HandleFunc("POST /api/v1/admin/qualifications/{id}/void", func(w http.ResponseWriter, r *http.Request) {
		reason, ok := reasonOf(w, r)
		if !ok {
			return
		}
		if err := s.VoidRun(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"), reason); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"status": "voided"})
	})

	mux.HandleFunc("POST /api/v1/admin/agents/{id}/ban", func(w http.ResponseWriter, r *http.Request) {
		reason, ok := reasonOf(w, r)
		if !ok {
			return
		}
		if err := s.BanAgent(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"), reason); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"status": "banned"})
	})

	mux.HandleFunc("POST /api/v1/admin/agents/{id}/unban", func(w http.ResponseWriter, r *http.Request) {
		reason, ok := reasonOf(w, r)
		if !ok {
			return
		}
		if err := s.UnbanAgent(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"), reason); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"status": "unbanned"})
	})
}
