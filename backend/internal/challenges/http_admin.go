package challenges

import (
	"context"
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

// payoutInput is the one thing an administrator edits after creation. Prizes are
// free text and paid outside the platform, so recording that they were paid is a
// note, not a ledger entry.
type payoutInput struct {
	Prizes     *string `json:"prizes"`
	PayoutNote *string `json:"payout_note"`
}

// RegisterAdminRoutes mounts the operator's challenge routes. They belong in the
// same mux as internal/admin's, behind identity.RequireAdmin.
func RegisterAdminRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/admin/challenges", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in NewInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		c, err := s.Create(r.Context(), identity.MustFromContext(r.Context()).UserID, in)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, c)
	})

	mux.HandleFunc("POST /api/v1/admin/challenges/{slug}/open", func(w http.ResponseWriter, r *http.Request) {
		transitionHandler(w, r, s.Open, "opened")
	})
	mux.HandleFunc("POST /api/v1/admin/challenges/{slug}/close", func(w http.ResponseWriter, r *http.Request) {
		transitionHandler(w, r, s.Close, "closed")
	})
	mux.HandleFunc("POST /api/v1/admin/challenges/{slug}/publish", func(w http.ResponseWriter, r *http.Request) {
		transitionHandler(w, r, s.Publish, "published")
	})

	mux.HandleFunc("PATCH /api/v1/admin/challenges/{slug}", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in payoutInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		c, err := s.Patch(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("slug"), in.Prizes, in.PayoutNote)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, c)
	})
}

func transitionHandler(w http.ResponseWriter, r *http.Request, fn func(context.Context, string, string) error, done string) {
	if err := fn(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("slug")); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.Respond(w, http.StatusOK, map[string]any{"status": done})
}
