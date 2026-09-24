package proofs

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

type createInput struct {
	TaskSlug string `json:"task_slug"`
}

func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/proof-tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks, err := s.Tasks(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": tasks})
	})
	mux.HandleFunc("POST /api/v1/proofs", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in createInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		p, err := s.Create(r.Context(), identity.MustFromContext(r.Context()).UserID, in.TaskSlug)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, p)
	})
	mux.HandleFunc("GET /api/v1/proofs", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.List(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/proofs/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Get(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
	mux.HandleFunc("POST /api/v1/proofs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Retry(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
}
