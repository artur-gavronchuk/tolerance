package agents

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idempotency"
)

type createKeyInput struct {
	Name string `json:"name"`
}

func RegisterMeRoutes(mux *http.ServeMux, pool *db.Pool, s *Service) {
	mux.HandleFunc("POST /api/v1/me/agent", idempotency.Command(pool, "POST /me/agent",
		func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
			var in CreateInput
			if err := httpx.Decode(raw, &in); err != nil {
				return nil, 0, err
			}
			p, err := s.Create(r.Context(), actor.UserID, in)
			return p, http.StatusCreated, err
		}))
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

func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/agents/{name}", func(w http.ResponseWriter, r *http.Request) {
		prof, st, err := s.ProfileByName(r.Context(), r.PathValue("name"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"profile": prof, "standing": st, "rank": st.Rank})
	})
}

func RegisterAgentRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/agent/me", func(w http.ResponseWriter, r *http.Request) {
		actor := identity.MustFromContext(r.Context())
		a, err := s.ByID(r.Context(), actor.AgentID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		st, err := s.standings.ForAgent(r.Context(), a.ID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var inQueue bool
		_ = s.pool.Tx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM arena_queue WHERE agent_id = $1)`, a.ID).Scan(&inQueue)
		})
		httpx.Respond(w, http.StatusOK, map[string]any{
			"agent":          map[string]any{"id": a.ID, "name": a.Name, "model": a.Model},
			"owner":          map[string]any{"handle": st.Author},
			"in_arena_queue": inQueue,
			"current_match":  nil, // slice 3
		})
	})
}
