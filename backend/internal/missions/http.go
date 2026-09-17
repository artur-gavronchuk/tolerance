package missions

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idempotency"
)

func RegisterRoutes(mux *http.ServeMux, pool *db.Pool, service *Service) {
	mux.HandleFunc("POST /api/v1/campaigns/{campaignId}/missions", idempotency.Command(pool, "POST /campaigns/{id}/missions",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var input CreateInput
			if err := httpx.Decode(raw, &input); err != nil {
				return nil, 0, err
			}
			m, err := service.Create(r.Context(), actor, r.PathValue("campaignId"), input)
			return m, http.StatusCreated, err
		}))
	mux.HandleFunc("GET /api/v1/missions/{id}", handleGet(service))
	mux.HandleFunc("POST /api/v1/missions/{id}/confirm-calibration", idempotency.Command(pool, "POST /missions/{id}/confirm-calibration",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			m, err := service.ConfirmCalibration(r.Context(), actor, r.PathValue("id"))
			return m, http.StatusOK, err
		}))
	mux.HandleFunc("POST /api/v1/missions/{id}/open", idempotency.Command(pool, "POST /missions/{id}/open",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var input openInput
			if err := httpx.Decode(raw, &input); err != nil {
				return nil, 0, err
			}
			m, err := service.Open(r.Context(), actor, r.PathValue("id"), input.ExpectedVersion)
			return m, http.StatusOK, err
		}))
}

func handleGet(service *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m, err := service.Get(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, m)
	}
}

type openInput struct {
	ExpectedVersion int `json:"expected_version"`
}
