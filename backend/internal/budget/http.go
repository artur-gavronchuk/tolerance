package budget

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idempotency"
)

func RegisterRoutes(mux *http.ServeMux, pool *db.Pool, service *Service) {
	mux.HandleFunc("POST /api/v1/campaigns/{id}/budget", idempotency.Command(pool, "POST /campaigns/{id}/budget",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var input SetInput
			if err := httpx.Decode(raw, &input); err != nil {
				return nil, 0, err
			}
			input.ScopeKind, input.ScopeID = "campaign", r.PathValue("id")
			b, err := service.Set(r.Context(), actor, input)
			return b, http.StatusOK, err
		}))
	mux.HandleFunc("GET /api/v1/campaigns/{id}/budget", handleGet(service))
}

func handleGet(service *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := service.Get(r.Context(), identity.MustFromContext(r.Context()), "campaign", r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, b)
	}
}
