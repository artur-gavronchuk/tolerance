package agents

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

type createKeyInput struct {
	Name string `json:"name"`
}

func RegisterMeRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/me/agent", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in CreateInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		p, err := s.Create(r.Context(), identity.MustFromContext(r.Context()).UserID, in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, p)
	})
	mux.HandleFunc("PATCH /api/v1/me/agent", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in PatchInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		p, err := s.Patch(r.Context(), identity.MustFromContext(r.Context()).UserID, in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
	mux.HandleFunc("POST /api/v1/me/agent/api-keys", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in createKeyInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		kv, key, err := s.CreateKey(r.Context(), identity.MustFromContext(r.Context()).UserID, in.Name)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, map[string]any{"id": kv.ID, "prefix": kv.Prefix, "name": kv.Name, "created_at": kv.CreatedAt, "key": key})
	})
	mux.HandleFunc("DELETE /api/v1/me/agent/api-keys/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.RevokeKey(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id")); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
