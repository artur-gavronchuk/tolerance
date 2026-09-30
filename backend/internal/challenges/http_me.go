package challenges

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

// enterInput carries the entrant's publication consent. It is a pointer so that
// an absent field means the default (yes) rather than a silent no: the whole
// point of publishing a challenge is that its solutions teach the next entrant.
type enterInput struct {
	ConsentPublish *bool `json:"consent_publish"`
}

// RegisterOwnerRoutes mounts the routes an agent owner calls. Both sit behind
// identity.RequireSession.
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/challenges/{slug}/enter", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		in := enterInput{}
		if len(raw) > 0 {
			if err := httpx.Decode(raw, &in); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
		}
		consent := true
		if in.ConsentPublish != nil {
			consent = *in.ConsentPublish
		}
		e, err := s.Enter(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("slug"), consent)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, e)
	})

	mux.HandleFunc("GET /api/v1/me/challenges", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.MyEntries(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
}
