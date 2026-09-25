package games

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

// RegisterConnectorRoutes registers the tanks arena's connector route: uploading a bot version on behalf of
// the agent's owner, the way `arena tanks submit` does.
func RegisterConnectorRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/connector/tanks/versions", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBodyLimit(w, r, maxVersionUploadBytes)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		archive, err := decodeArchiveBody(raw)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		v, err := s.UploadVersionForAgent(r.Context(), identity.MustFromContext(r.Context()).AgentID, archive)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, v)
	})
}
