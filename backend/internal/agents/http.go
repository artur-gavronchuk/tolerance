package agents

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

type createKeyInput struct {
	Name string `json:"name"`
}

func decode[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var in T
	raw, err := httpx.ReadBody(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return in, false
	}
	if err := httpx.Decode(raw, &in); err != nil {
		httpx.WriteError(w, r, err)
		return in, false
	}
	return in, true
}

// RegisterOwnerRoutes mounts the cabinet's agent routes (cookie session).
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/agent", func(w http.ResponseWriter, r *http.Request) {
		in, ok := decode[CreateInput](w, r)
		if !ok {
			return
		}
		p, err := s.Create(r.Context(), identity.MustFromContext(r.Context()).UserID, in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, p)
	})
	mux.HandleFunc("PATCH /api/v1/agent", func(w http.ResponseWriter, r *http.Request) {
		in, ok := decode[PatchInput](w, r)
		if !ok {
			return
		}
		p, err := s.Patch(r.Context(), identity.MustFromContext(r.Context()).UserID, in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
	mux.HandleFunc("POST /api/v1/agent/keys", func(w http.ResponseWriter, r *http.Request) {
		in, ok := decode[createKeyInput](w, r)
		if !ok {
			return
		}
		kv, key, err := s.CreateKey(r.Context(), identity.MustFromContext(r.Context()).UserID, in.Name)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, map[string]any{"id": kv.ID, "prefix": kv.Prefix, "name": kv.Name, "created_at": kv.CreatedAt, "key": key})
	})
	mux.HandleFunc("DELETE /api/v1/agent/keys/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.RevokeKey(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id")); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// RegisterConnectorRoutes mounts the connector's heartbeat (API key).
func RegisterConnectorRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/connector/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		in, ok := decode[heartbeatInput](w, r)
		if !ok {
			return
		}
		agentID := identity.MustFromContext(r.Context()).AgentID
		if err := s.Heartbeat(r.Context(), agentID, in.ConnectorVersion, in.Hostname); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		o, err := s.OverviewByID(r.Context(), agentID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"agent": map[string]any{"id": o.ID, "name": o.Name, "stage": o.Stage}})
	})
}
