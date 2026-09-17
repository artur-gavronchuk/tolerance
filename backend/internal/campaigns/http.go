package campaigns

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idempotency"
)

func RegisterRoutes(mux *http.ServeMux, pool *db.Pool, service *Service) {
	mux.HandleFunc("POST /api/v1/campaigns", idempotency.Command(pool, "POST /campaigns",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var input CreateInput
			if err := httpx.Decode(raw, &input); err != nil {
				return nil, 0, err
			}
			c, err := service.Create(r.Context(), actor, input)
			return c, http.StatusCreated, err
		}))
	mux.HandleFunc("GET /api/v1/campaigns", handleList(service))
	mux.HandleFunc("GET /api/v1/campaigns/{id}", handleGet(service))
	mux.HandleFunc("POST /api/v1/campaigns/{id}/activate", idempotency.Command(pool, "POST /campaigns/{id}/activate",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var input transitionInput
			if err := httpx.Decode(raw, &input); err != nil {
				return nil, 0, err
			}
			c, err := service.Activate(r.Context(), actor, r.PathValue("id"), input.ExpectedVersion)
			return c, http.StatusOK, err
		}))
	mux.HandleFunc("POST /api/v1/campaigns/{id}/complete", idempotency.Command(pool, "POST /campaigns/{id}/complete",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var input transitionInput
			if err := httpx.Decode(raw, &input); err != nil {
				return nil, 0, err
			}
			c, err := service.Complete(r.Context(), actor, r.PathValue("id"), input.ExpectedVersion)
			return c, http.StatusOK, err
		}))
	mux.HandleFunc("POST /api/v1/campaigns/{id}/cancel", idempotency.Command(pool, "POST /campaigns/{id}/cancel",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var input transitionInput
			if err := httpx.Decode(raw, &input); err != nil {
				return nil, 0, err
			}
			c, err := service.Cancel(r.Context(), actor, r.PathValue("id"), input.ExpectedVersion, input.Reason)
			return c, http.StatusOK, err
		}))
}

func handleList(service *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := service.List(r.Context(), identity.MustFromContext(r.Context()))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, list)
	}
}

func handleGet(service *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := service.Get(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, c)
	}
}

type transitionInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Reason          string `json:"reason,omitempty"`
}
