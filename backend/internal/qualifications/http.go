package qualifications

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

type startInput struct {
	Skill string `json:"skill"`
}

func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/qualifications", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in startInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		run, err := s.Start(r.Context(), identity.MustFromContext(r.Context()).UserID, in.Skill)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, run)
	})
	mux.HandleFunc("GET /api/v1/qualifications", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.List(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/qualifications/{id}", func(w http.ResponseWriter, r *http.Request) {
		run, err := s.Get(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, run)
	})
}
