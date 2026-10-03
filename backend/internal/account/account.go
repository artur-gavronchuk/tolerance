// Package account is the privacy side of a user: "download my data" and "delete my account".
//
// Deletion runs in one transaction and decides per table:
//   - personal data (identities, sessions, upload link, notifications, email, handle): deleted or replaced;
//   - what the user made for the daily task (submissions): deleted, so leaderboards and streaks recompute;
//   - tank bots: deactivated (hidden_at) and anonymized (name, source archives, logs, season owner label), but
//     the bot, its versions and match_players rows stay so other players' past matches and replays still
//     open, showing a "Deleted player";
//   - the users row stays as a tombstone (handle "deleted-<id>", placeholder email, banned_at and deleted_at
//     set) because bots reference it;
//   - audit_events are append-only history: handle labels in the user's own events are scrubbed, the opaque
//     user id remains, and one account.delete event is added.
package account

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

// exportQueries: name -> SQL returning one jsonb value for $1 = user id. Secrets (session ids, upload token
// hashes) are left out; binary columns come out as base64.
var exportQueries = []struct{ key, sql string }{
	{"user", `SELECT to_jsonb(u) FROM users u WHERE u.id = $1`},
	{"identities", `SELECT coalesce(jsonb_agg(to_jsonb(i) ORDER BY i.created_at), '[]') FROM user_identities i WHERE i.user_id = $1`},
	{"sessions", `SELECT coalesce(jsonb_agg((to_jsonb(s) - 'id') ORDER BY s.created_at), '[]') FROM sessions s WHERE s.user_id = $1`},
	{"upload_link", `SELECT coalesce((SELECT to_jsonb(l) - 'token_hash' FROM upload_links l WHERE l.user_id = $1), 'null')`},
	{"submissions", `SELECT coalesce(jsonb_agg(to_jsonb(s) ORDER BY s.created_at), '[]') FROM submissions s WHERE s.user_id = $1`},
	{"notifications", `SELECT coalesce(jsonb_agg(to_jsonb(n) ORDER BY n.created_at), '[]') FROM notifications n WHERE n.user_id = $1`},
	{"notify_state", `SELECT coalesce((SELECT to_jsonb(n) FROM notify_state n WHERE n.user_id = $1), 'null')`},
	{"tank_bots", `SELECT coalesce(jsonb_agg(to_jsonb(b) ORDER BY b.created_at), '[]') FROM game_bots b WHERE b.owner_user_id = $1`},
	{"tank_bot_versions", `SELECT coalesce(jsonb_agg((to_jsonb(v) - 'archive') || jsonb_build_object('archive_base64', encode(v.archive, 'base64')) ORDER BY v.created_at), '[]')
		FROM bot_versions v JOIN game_bots b ON b.id = v.bot_id WHERE b.owner_user_id = $1`},
	{"tank_match_players", `SELECT coalesce(jsonb_agg(to_jsonb(p) ORDER BY p.match_id), '[]')
		FROM match_players p JOIN game_bots b ON b.id = p.bot_id WHERE b.owner_user_id = $1`},
	{"audit_events", `SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY a.at), '[]') FROM audit_events a
		WHERE a.actor_id = $1 OR (a.aggregate_kind = 'user' AND a.aggregate_id = $1)`},
}

// Export collects everything stored about the user as one JSON-ready map.
func (s *Service) Export(ctx context.Context, userID string) (map[string]any, error) {
	out := map[string]any{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, q := range exportQueries {
			var v any
			if err := tx.QueryRow(ctx, q.sql, userID).Scan(&v); err != nil {
				return err
			}
			out[q.key] = v
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out["exported_at"] = time.Now().UTC()
	return out, nil
}

// Delete erases the account; confirmHandle must equal the user's handle (case-insensitive).
func (s *Service) Delete(ctx context.Context, userID, confirmHandle string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var handle string
		if err := tx.QueryRow(ctx, `SELECT handle FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&handle); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(confirmHandle), handle) {
			return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "Type your handle to confirm", "handle", "mismatch")
		}
		for _, q := range []string{
			// Queued runs of what is about to disappear.
			`DELETE FROM jobs WHERE state = 'queued' AND (
				payload->>'submission_id' IN (SELECT id FROM submissions WHERE user_id = $1) OR
				payload->>'version_id' IN (SELECT v.id FROM bot_versions v JOIN game_bots b ON b.id = v.bot_id WHERE b.owner_user_id = $1))`,
			`DELETE FROM submissions WHERE user_id = $1`,
			`DELETE FROM notifications WHERE user_id = $1`,
			`DELETE FROM notify_state WHERE user_id = $1`,
			`DELETE FROM upload_links WHERE user_id = $1`,
			`DELETE FROM sessions WHERE user_id = $1`,
			`DELETE FROM user_identities WHERE user_id = $1`,
			// Tank bots: keep rows for other players' match history, drop everything of the person.
			`UPDATE tanks_season_standings SET owner = '' WHERE bot_id IN (SELECT id FROM game_bots WHERE owner_user_id = $1)`,
			`UPDATE match_players SET stderr_tail = '' WHERE bot_id IN (SELECT id FROM game_bots WHERE owner_user_id = $1)`,
			`UPDATE bot_versions SET archive = ''::bytea, check_log = '', checks = '[]'::jsonb
				WHERE bot_id IN (SELECT id FROM game_bots WHERE owner_user_id = $1)`,
			`UPDATE game_bots SET name = 'Deleted player ' || substr(id, length(split_part(id, '_', 1)) + 2), hidden_at = now()
				WHERE owner_user_id = $1`,
			`UPDATE audit_events SET payload = payload - 'label' WHERE aggregate_kind = 'user' AND aggregate_id = $1`,
			`UPDATE users SET email = 'deleted-' || id || '@deleted.invalid', handle = 'deleted-' || id, role = 'user',
				banned_at = coalesce(banned_at, now()), deleted_at = now() WHERE id = $1`,
		} {
			if _, err := tx.Exec(ctx, q, userID); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "account.delete", AggregateKind: "user", AggregateID: userID,
			RequestID: httpx.RequestID(ctx)})
	})
}

// RegisterOwnerRoutes mounts DELETE /me and GET /me/export (session required). secure is the cookie Secure flag.
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service, secure bool) {
	mux.HandleFunc("GET /api/v1/me/export", func(w http.ResponseWriter, r *http.Request) {
		data, err := s.Export(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="tolerance-my-data.json"`)
		httpx.Respond(w, http.StatusOK, data)
	})
	mux.HandleFunc("DELETE /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in struct {
			Handle string `json:"handle"`
		}
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if err := s.Delete(r.Context(), identity.MustFromContext(r.Context()).UserID, in.Handle); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		identity.ClearSessionCookie(w, secure)
		w.WriteHeader(http.StatusNoContent)
	})
}
