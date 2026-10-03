// Package discussion is the comment thread under a closed daily task. It opens when the day closes, so nobody can
// share a solution while the task is live; the check is server-side, on every route.
package discussion

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/daily"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/limits"
	"tolerance/internal/platform/ratelimit"
)

const (
	MaxBody    = 4000
	EditWindow = 15 * time.Minute
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

type Comment struct {
	ID        string     `json:"id"`
	Handle    string     `json:"handle"`
	MadeWith  string     `json:"made_with"` // of the author's best submission that day; "" = none or not stated
	Passed    *int       `json:"passed_tests"`
	Total     *int       `json:"total_tests"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
	EditedAt  *time.Time `json:"edited_at"`
	Mine      bool       `json:"mine"`
	CanEdit   bool       `json:"can_edit"` // mine and still inside the edit window
}

// requireClosed: unknown or future day = 404, today (still open) = 403 day_open.
func requireClosed(day string) error {
	if _, ok := daily.ParseDay(day); !ok || day > daily.Today() {
		return httpx.NotFound()
	}
	if day == daily.Today() {
		return daily.ErrNotClosed
	}
	return nil
}

func clean(body string) (string, error) {
	body = strings.ToValidUTF8(body, "")
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")
	body = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, body)
	body = strings.TrimSpace(body)
	if body == "" {
		return "", httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "Write something first", "body", "required")
	}
	if utf8.RuneCountInString(body) > MaxBody {
		return "", httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "The comment is too long", "body", "too_long")
	}
	return body, nil
}

func (s *Service) List(ctx context.Context, day, userID string, offset, limit int) ([]Comment, int, error) {
	if err := requireClosed(day); err != nil {
		return nil, 0, err
	}
	out := []Comment{}
	total := 0
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM discussion_comments c JOIN users u ON u.id = c.user_id
			WHERE c.day = $1 AND c.hidden_at IS NULL AND u.banned_at IS NULL`, day).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT c.id, u.handle, c.user_id, coalesce(b.made_with, ''), b.passed_tests, b.total_tests, c.body, c.created_at, c.edited_at
			FROM discussion_comments c
			JOIN users u ON u.id = c.user_id
			LEFT JOIN LATERAL (
				SELECT s.made_with, s.passed_tests, s.total_tests FROM submissions s
				WHERE s.user_id = c.user_id AND s.day = c.day AND s.status IN ('passed', 'failed') AND s.hidden_at IS NULL
				ORDER BY s.passed_tests DESC, s.created_at ASC LIMIT 1) b ON true
			WHERE c.day = $1 AND c.hidden_at IS NULL AND u.banned_at IS NULL
			ORDER BY c.created_at, c.id OFFSET $2 LIMIT $3`, day, offset, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		now := time.Now()
		for rows.Next() {
			var c Comment
			var owner string
			if err := rows.Scan(&c.ID, &c.Handle, &owner, &c.MadeWith, &c.Passed, &c.Total, &c.Body, &c.CreatedAt, &c.EditedAt); err != nil {
				return err
			}
			c.CreatedAt = c.CreatedAt.UTC()
			if c.EditedAt != nil {
				t := c.EditedAt.UTC()
				c.EditedAt = &t
			}
			c.Mine = userID != "" && owner == userID
			c.CanEdit = c.Mine && now.Sub(c.CreatedAt) < EditWindow
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, total, err
}

func (s *Service) Post(ctx context.Context, day, userID, body string) (string, error) {
	if err := requireClosed(day); err != nil {
		return "", err
	}
	body, err := clean(body)
	if err != nil {
		return "", err
	}
	id := idgen.New("cm")
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM daily_tasks WHERE day = $1::date)`, day).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.NotFound()
		}
		_, err := tx.Exec(ctx, `INSERT INTO discussion_comments (id, day, user_id, body) VALUES ($1, $2::date, $3, $4)`, id, day, userID, body)
		return err
	})
	return id, err
}

// own loads a comment for its author and checks the edit window.
func own(ctx context.Context, tx pgx.Tx, id, userID string) error {
	var owner string
	var created time.Time
	var day string
	if err := tx.QueryRow(ctx, `SELECT user_id, created_at, day::text FROM discussion_comments WHERE id = $1 AND hidden_at IS NULL FOR UPDATE`, id).Scan(&owner, &created, &day); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		return err
	}
	if owner != userID {
		return httpx.Forbidden("You can only change your own comments")
	}
	if time.Since(created) >= EditWindow {
		return httpx.New(http.StatusForbidden, "edit_window_closed", "A comment can be changed for 15 minutes after posting")
	}
	return nil
}

func (s *Service) Edit(ctx context.Context, id, userID, body string) error {
	body, err := clean(body)
	if err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := own(ctx, tx, id, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE discussion_comments SET body = $2, edited_at = now() WHERE id = $1`, id, body)
		return err
	})
}

func (s *Service) Delete(ctx context.Context, id, userID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := own(ctx, tx, id, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM discussion_comments WHERE id = $1`, id)
		return err
	})
}

func rateLimited(limiter *ratelimit.Limiter, uid string) bool {
	return !limiter.Allow("comment:"+uid, limits.Cap(10), time.Hour) || !limiter.Allow("comment-day:"+uid, limits.Cap(40), 24*time.Hour)
}

// RegisterPublicRoutes mounts the thread read; who gives the viewer's id ("" = anonymous).
func RegisterPublicRoutes(mux *http.ServeMux, s *Service, who func(r *http.Request) string) {
	mux.HandleFunc("GET /api/v1/daily/{day}/comments", func(w http.ResponseWriter, r *http.Request) {
		offset, limit, err := httpx.PageParams(r, 100, 200)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		items, total, err := s.List(r.Context(), r.PathValue("day"), who(r), offset, limit)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items, "total": total})
	})
}

// RegisterOwnerRoutes mounts the writes (session required).
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service, limiter *ratelimit.Limiter) {
	uid := func(r *http.Request) string { return identity.MustFromContext(r.Context()).UserID }
	read := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		var b struct {
			Body string `json:"body"`
		}
		raw, err := httpx.ReadBody(w, r)
		if err == nil {
			err = httpx.Decode(raw, &b)
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return "", false
		}
		return b.Body, true
	}
	ok := func(w http.ResponseWriter, r *http.Request, err error) {
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]bool{"ok": true})
	}
	mux.HandleFunc("POST /api/v1/daily/{day}/comments", func(w http.ResponseWriter, r *http.Request) {
		if rateLimited(limiter, uid(r)) {
			httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "You are commenting too fast; try again later"))
			return
		}
		body, good := read(w, r)
		if !good {
			return
		}
		id, err := s.Post(r.Context(), r.PathValue("day"), uid(r), body)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, map[string]string{"id": id})
	})
	mux.HandleFunc("PATCH /api/v1/comments/{id}", func(w http.ResponseWriter, r *http.Request) {
		if body, good := read(w, r); good {
			ok(w, r, s.Edit(r.Context(), r.PathValue("id"), uid(r), body))
		}
	})
	mux.HandleFunc("DELETE /api/v1/comments/{id}", func(w http.ResponseWriter, r *http.Request) {
		ok(w, r, s.Delete(r.Context(), r.PathValue("id"), uid(r)))
	})
}
