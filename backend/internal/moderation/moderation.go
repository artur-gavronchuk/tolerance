// Package moderation lets admins ban users and hide single daily submissions and tank bots.
// The flags live in users.banned_at and <table>.hidden_at; public queries filter on them. A ban cascades
// hidden_at (hidden_by_ban) onto the user's content, and an unban lifts only that. Every action is audited.
package moderation

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

// kinds maps the API name of a hideable thing to its table.
var kinds = map[string]string{"submission": "submissions", "bot": "game_bots", "comment": "discussion_comments"}

const maxReason = 500

type Actor struct{ UserID string }

func cleanReason(r string) (string, error) {
	r = strings.TrimSpace(r)
	if r == "" {
		return "", httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "A reason is required", "reason", "required")
	}
	if len([]rune(r)) > maxReason {
		return "", httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "The reason is too long", "reason", "too_long")
	}
	return r, nil
}

func (s *Service) Ban(ctx context.Context, actorID, userID, reason string) error {
	reason, err := cleanReason(reason)
	if err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var handle, role string
		var banned bool
		if err := tx.QueryRow(ctx, `SELECT handle, role, banned_at IS NOT NULL FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&handle, &role, &banned); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		if userID == actorID || role == "admin" {
			return httpx.Forbidden("Admins cannot be banned")
		}
		if banned {
			return httpx.New(http.StatusConflict, "state_conflict", "The user is already banned")
		}
		for _, q := range []string{
			`UPDATE users SET banned_at = now() WHERE id = $1`,
			`DELETE FROM sessions WHERE user_id = $1`,
			`DELETE FROM upload_links WHERE user_id = $1`,
			`UPDATE submissions SET hidden_at = now(), hidden_by_ban = true WHERE user_id = $1 AND hidden_at IS NULL`,
			`UPDATE game_bots SET hidden_at = now(), hidden_by_ban = true WHERE owner_user_id = $1 AND hidden_at IS NULL`,
			`UPDATE discussion_comments SET hidden_at = now(), hidden_by_ban = true WHERE user_id = $1 AND hidden_at IS NULL`,
		} {
			if _, err := tx.Exec(ctx, q, userID); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, Action: "moderation.ban", AggregateKind: "user", AggregateID: userID,
			Reason: reason, RequestID: httpx.RequestID(ctx), Payload: map[string]string{"label": handle}})
	})
}

func (s *Service) Unban(ctx context.Context, actorID, userID, reason string) error {
	reason, err := cleanReason(reason)
	if err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var handle string
		err := tx.QueryRow(ctx, `UPDATE users SET banned_at = NULL WHERE id = $1 AND banned_at IS NOT NULL RETURNING handle`, userID).Scan(&handle)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.New(http.StatusConflict, "state_conflict", "The user is not banned")
		}
		if err != nil {
			return err
		}
		for _, q := range []string{
			`UPDATE submissions SET hidden_at = NULL, hidden_by_ban = false WHERE user_id = $1 AND hidden_by_ban`,
			`UPDATE game_bots SET hidden_at = NULL, hidden_by_ban = false WHERE owner_user_id = $1 AND hidden_by_ban`,
			`UPDATE discussion_comments SET hidden_at = NULL, hidden_by_ban = false WHERE user_id = $1 AND hidden_by_ban`,
		} {
			if _, err := tx.Exec(ctx, q, userID); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, Action: "moderation.unban", AggregateKind: "user", AggregateID: userID,
			Reason: reason, RequestID: httpx.RequestID(ctx), Payload: map[string]string{"label": handle}})
	})
}

// labelSQL describes a hideable row for the audit log: who owns it and what it is.
var labelSQL = map[string]string{
	"submission": `SELECT u.handle || ' / ' || coalesce(s.day::text, 'practice') FROM submissions s JOIN users u ON u.id = s.user_id WHERE s.id = $1`,
	"bot":        `SELECT g.name FROM game_bots g WHERE g.id = $1`,
	"comment":    `SELECT u.handle || ' / ' || c.day::text || ': ' || left(c.body, 60) FROM discussion_comments c JOIN users u ON u.id = c.user_id WHERE c.id = $1`,
}

func (s *Service) SetHidden(ctx context.Context, actorID, kind, id, reason string, hide bool) error {
	table, ok := kinds[kind]
	if !ok {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "kind must be submission, bot or comment", "kind", "invalid")
	}
	reason, err := cleanReason(reason)
	if err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var label string
		if err := tx.QueryRow(ctx, labelSQL[kind], id).Scan(&label); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		set := `UPDATE ` + table + ` SET hidden_at = now(), hidden_by_ban = false WHERE id = $1 AND hidden_at IS NULL`
		action := "moderation.hide"
		if !hide {
			set = `UPDATE ` + table + ` SET hidden_at = NULL, hidden_by_ban = false WHERE id = $1 AND hidden_at IS NOT NULL`
			action = "moderation.unhide"
		}
		tag, err := tx.Exec(ctx, set, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.New(http.StatusConflict, "state_conflict", "That is already in the requested state")
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, Action: action, AggregateKind: kind, AggregateID: id,
			Reason: reason, RequestID: httpx.RequestID(ctx), Payload: map[string]string{"label": label}})
	})
}

type UserRow struct {
	ID          string     `json:"id"`
	Handle      string     `json:"handle"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	CreatedAt   time.Time  `json:"created_at"`
	BannedAt    *time.Time `json:"banned_at"`
	Submissions int        `json:"submissions"`
}

// SearchUsers finds users by a handle or email fragment; an empty query lists the newest.
func (s *Service) SearchUsers(ctx context.Context, q string) ([]UserRow, error) {
	q = strings.TrimSpace(q)
	out := []UserRow{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT u.id, u.handle, u.email, u.role, u.created_at, u.banned_at,
			       (SELECT count(*) FROM submissions s WHERE s.user_id = u.id)::int
			FROM users u
			WHERE $1 = '' OR u.handle ILIKE '%' || $1 || '%' OR u.email ILIKE '%' || $1 || '%'
			ORDER BY (u.banned_at IS NOT NULL) DESC, u.created_at DESC LIMIT 20`, escapeLike(q))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r UserRow
			if err := rows.Scan(&r.ID, &r.Handle, &r.Email, &r.Role, &r.CreatedAt, &r.BannedAt, &r.Submissions); err != nil {
				return err
			}
			r.CreatedAt = r.CreatedAt.UTC()
			if r.BannedAt != nil {
				t := r.BannedAt.UTC()
				r.BannedAt = &t
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

func escapeLike(q string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
}

// Item is one hideable thing owned by a user.
type Item struct {
	Kind     string     `json:"kind"`
	ID       string     `json:"id"`
	Label    string     `json:"label"`
	Status   string     `json:"status"`
	At       time.Time  `json:"at"`
	HiddenAt *time.Time `json:"hidden_at"`
}

// ItemsOf lists a user's latest submissions and bots.
func (s *Service) ItemsOf(ctx context.Context, userID string) ([]Item, error) {
	out := []Item{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			(SELECT 'submission', id, coalesce(day::text, 'practice') || ' / ' || task_slug, status, created_at, hidden_at
			   FROM submissions WHERE user_id = $1 ORDER BY created_at DESC LIMIT 20)
			UNION ALL
			(SELECT 'bot', id, name, CASE WHEN active_version_id IS NULL THEN 'no active version' ELSE 'active' END, created_at, hidden_at
			   FROM game_bots WHERE owner_user_id = $1)
			ORDER BY 5 DESC`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var it Item
			if err := rows.Scan(&it.Kind, &it.ID, &it.Label, &it.Status, &it.At, &it.HiddenAt); err != nil {
				return err
			}
			it.At = it.At.UTC()
			if it.HiddenAt != nil {
				t := it.HiddenAt.UTC()
				it.HiddenAt = &t
			}
			out = append(out, it)
		}
		return rows.Err()
	})
	return out, err
}

type LogItem struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"` // ban | unban | hide | unhide
	Kind   string    `json:"kind"`   // user | submission | bot
	ID     string    `json:"id"`
	Label  string    `json:"label"`
	Actor  string    `json:"actor"`
	Reason string    `json:"reason"`
	// Active: the ban or hide is still in force (only set for ban and hide rows).
	Active bool `json:"active"`
}

// Log is the newest moderation actions, with who did them and why.
func (s *Service) Log(ctx context.Context) ([]LogItem, error) {
	out := []LogItem{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.at, substr(a.action, 12), a.aggregate_kind, a.aggregate_id, coalesce(a.payload->>'label', ''),
			       coalesce(u.handle, a.actor_id), coalesce(a.reason, ''),
			       CASE a.action
			         WHEN 'moderation.ban' THEN coalesce((SELECT banned_at IS NOT NULL FROM users WHERE id = a.aggregate_id), false)
			         WHEN 'moderation.hide' THEN coalesce(CASE a.aggregate_kind
			           WHEN 'submission' THEN (SELECT hidden_at IS NOT NULL FROM submissions WHERE id = a.aggregate_id)
			           WHEN 'bot' THEN (SELECT hidden_at IS NOT NULL FROM game_bots WHERE id = a.aggregate_id)
           WHEN 'comment' THEN (SELECT hidden_at IS NOT NULL FROM discussion_comments WHERE id = a.aggregate_id) END, false)
			         ELSE false END
			FROM audit_events a LEFT JOIN users u ON u.id = a.actor_id
			WHERE a.action LIKE 'moderation.%' ORDER BY a.at DESC LIMIT 50`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var l LogItem
			if err := rows.Scan(&l.At, &l.Action, &l.Kind, &l.ID, &l.Label, &l.Actor, &l.Reason, &l.Active); err != nil {
				return err
			}
			l.At = l.At.UTC()
			out = append(out, l)
		}
		return rows.Err()
	})
	return out, err
}

// RegisterRoutes mounts the moderation routes; the caller wraps them in the session and admin guards.
func RegisterRoutes(mux *http.ServeMux, s *Service) {
	type reasonBody struct {
		Reason string `json:"reason"`
	}
	type hideBody struct {
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	body := func(w http.ResponseWriter, r *http.Request, dst any) bool {
		raw, err := httpx.ReadBody(w, r)
		if err == nil {
			err = httpx.Decode(raw, dst)
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return false
		}
		return true
	}
	done := func(w http.ResponseWriter, r *http.Request, err error) {
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]bool{"ok": true})
	}
	actor := func(r *http.Request) string { return identity.MustFromContext(r.Context()).UserID }

	mux.HandleFunc("GET /api/v1/admin/moderation/users", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.SearchUsers(r.Context(), r.URL.Query().Get("q"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/admin/moderation/users/{id}/items", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.ItemsOf(r.Context(), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /api/v1/admin/moderation/log", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.Log(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /api/v1/admin/moderation/users/{id}/ban", func(w http.ResponseWriter, r *http.Request) {
		var b reasonBody
		if body(w, r, &b) {
			done(w, r, s.Ban(r.Context(), actor(r), r.PathValue("id"), b.Reason))
		}
	})
	mux.HandleFunc("POST /api/v1/admin/moderation/users/{id}/unban", func(w http.ResponseWriter, r *http.Request) {
		var b reasonBody
		if body(w, r, &b) {
			done(w, r, s.Unban(r.Context(), actor(r), r.PathValue("id"), b.Reason))
		}
	})
	mux.HandleFunc("POST /api/v1/admin/moderation/hide", func(w http.ResponseWriter, r *http.Request) {
		var b hideBody
		if body(w, r, &b) {
			done(w, r, s.SetHidden(r.Context(), actor(r), b.Kind, b.ID, b.Reason, true))
		}
	})
	mux.HandleFunc("POST /api/v1/admin/moderation/unhide", func(w http.ResponseWriter, r *http.Request) {
		var b hideBody
		if body(w, r, &b) {
			done(w, r, s.SetHidden(r.Context(), actor(r), b.Kind, b.ID, b.Reason, false))
		}
	})
}
