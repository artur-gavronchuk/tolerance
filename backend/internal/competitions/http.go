package competitions

import (
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idempotency"
)

type transitionInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Reason          string `json:"reason,omitempty"`
}

func RegisterAdminRoutes(mux *http.ServeMux, pool *db.Pool, s *Service) {
	mux.HandleFunc("GET /api/v1/admin/competitions", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.ListAdmin(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items := make([]AdminView, 0, len(list))
		for _, c := range list {
			items = append(items, c.Admin())
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /api/v1/admin/competitions", idempotency.Command(pool, "POST /admin/competitions",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var in Input
			if err := httpx.Decode(raw, &in); err != nil {
				return nil, 0, err
			}
			c, err := s.Create(r.Context(), actor, in)
			if err != nil {
				return nil, 0, err
			}
			return c.Admin(), http.StatusCreated, nil
		}))
	mux.HandleFunc("PATCH /api/v1/admin/competitions/{id}", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in Input
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		c, err := s.Update(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id"), in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, c.Admin())
	})
	mux.HandleFunc("DELETE /api/v1/admin/competitions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Delete(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id")); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/admin/competitions/{id}/publish", func(w http.ResponseWriter, r *http.Request) {
		var in transitionInput
		if err := readInto(w, r, &in); err != nil {
			return
		}
		c, err := s.Publish(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id"), in.ExpectedVersion)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, c.Admin())
	})
	mux.HandleFunc("POST /api/v1/admin/competitions/{id}/close", func(w http.ResponseWriter, r *http.Request) {
		var in transitionInput
		if err := readInto(w, r, &in); err != nil {
			return
		}
		c, err := s.Close(r.Context(), identity.MustFromContext(r.Context()), r.PathValue("id"), in.ExpectedVersion, in.Reason)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, c.Admin())
	})
}

// readInto reads and decodes the body, writing the error response itself
// so handlers can `return` on a non-nil result.
func readInto(w http.ResponseWriter, r *http.Request, dst any) error {
	raw, err := httpx.ReadBody(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return err
	}
	if err := httpx.Decode(raw, dst); err != nil {
		httpx.WriteError(w, r, err)
		return err
	}
	return nil
}

func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/competitions", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.ListPublic(r.Context(), r.URL.Query().Get("status"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items := make([]PublicView, 0, len(list))
		for _, c := range list {
			items = append(items, c.Public())
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/competitions/{slug}", func(w http.ResponseWriter, r *http.Request) {
		c, err := s.GetPublicBySlug(r.Context(), r.PathValue("slug"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, c.Public())
	})
}
