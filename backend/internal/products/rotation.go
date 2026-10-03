package products

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/httpx"
)

// The weekly rotation: weeks start Monday 00:00 UTC and exactly one task is "this week's". Like internal/daily
// it is lazy: the first List or worker tick of a week that finds no task started in it picks the next one.
// A task's run window lives on its product_tasks row (opens_at, deadline); a NULL window means never played.

func weekStart(t time.Time) time.Time {
	t = t.UTC()
	back := (int(t.Weekday()) + 6) % 7 // Monday = 0
	return time.Date(t.Year(), t.Month(), t.Day()-back, 0, 0, 0, 0, time.UTC)
}

// Upcoming is what the public sees of tasks that have not been played yet: how many, and the kind of the next.
type Upcoming struct {
	Count      int       `json:"count"`
	NextKind   string    `json:"next_kind,omitempty"`
	NextOpenAt time.Time `json:"next_opens_at"`
}

// pickNext chooses the task for the next run: never-played tasks first, alternating kinds (site first, then
// cli, site, ...) when the preferred kind still has a fresh task, then by manifest order and slug. When every
// task has been played the least recently played one comes back.
func pickNext(ctx context.Context, tx pgx.Tx) (slug, kind string, ok bool, err error) {
	want := KindSite
	var last string
	switch err := tx.QueryRow(ctx, `SELECT kind FROM product_tasks WHERE opens_at IS NOT NULL ORDER BY opens_at DESC, slug LIMIT 1`).Scan(&last); {
	case err == nil:
		if last == KindSite {
			want = KindCLI
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return "", "", false, err
	}
	err = tx.QueryRow(ctx, `SELECT slug, kind FROM product_tasks WHERE active AND NOT frozen
		ORDER BY (opens_at IS NOT NULL), (kind <> $1), opens_at, ord, slug LIMIT 1`, want).Scan(&slug, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	return slug, kind, err == nil, err
}

func upcoming(ctx context.Context, tx pgx.Tx) (Upcoming, error) {
	u := Upcoming{NextOpenAt: weekStart(time.Now()).AddDate(0, 0, 7)}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM product_tasks WHERE active AND NOT frozen AND opens_at IS NULL`).Scan(&u.Count); err != nil {
		return u, err
	}
	_, kind, ok, err := pickNext(ctx, tx)
	if ok {
		u.NextKind = kind
	}
	return u, err
}

// EnsureWeek starts this week's task when none has started in the current week yet.
func (s *Service) EnsureWeek(ctx context.Context) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := rotate(ctx, tx, false, "")
		return err
	})
}

// StartNext (admin, local runs) ends uploads of any open task and starts the next one right now, so the whole
// cycle can be tried without waiting for Monday. It returns the new task's slug.
func (s *Service) StartNext(ctx context.Context, actorID string) (string, error) {
	var slug string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		slug, err = rotate(ctx, tx, true, actorID)
		return err
	})
	if err == nil && slug == "" {
		return "", httpx.New(http.StatusConflict, "no_tasks", "The catalog has no active product tasks")
	}
	return slug, err
}

// rotate starts the next run and returns its slug ("" when nothing was started). Without force it only acts
// when no task has started since this Monday; with force it always acts and closes the open task first.
func rotate(ctx context.Context, tx pgx.Tx, force bool, actorID string) (string, error) {
	now := time.Now().UTC()
	ws := weekStart(now)
	started := func() (bool, error) {
		var b bool
		err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM product_tasks WHERE opens_at >= $1)`, ws).Scan(&b)
		return b, err
	}
	if !force {
		if b, err := started(); err != nil || b {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('products:rotate'))`); err != nil {
		return "", err
	}
	if !force {
		if b, err := started(); err != nil || b {
			return "", err
		}
	} else if _, err := tx.Exec(ctx, `UPDATE product_tasks SET deadline = $1 WHERE opens_at <= $1 AND deadline > $1`, now); err != nil {
		return "", err
	}
	slug, _, ok, err := pickNext(ctx, tx)
	if err != nil || !ok {
		return "", err
	}
	opens := ws
	if force {
		opens = now
	}
	var prev *time.Time
	if err := tx.QueryRow(ctx, `SELECT opens_at FROM product_tasks WHERE slug = $1`, slug).Scan(&prev); err != nil {
		return "", err
	}
	if prev != nil {
		// A recycled task: its earlier run (entries, votes) moves to a frozen copy so the new run starts clean.
		old := slug + "-w" + prev.UTC().Format("20060102-150405")
		if _, err := tx.Exec(ctx, `INSERT INTO product_tasks (slug, title, summary, kind, task_md, image, command, timeout_s, scenarios,
				opens_at, deadline, active, frozen, ord)
			SELECT $2, title, summary, kind, task_md, image, command, timeout_s, scenarios, opens_at, deadline, true, true, ord
			FROM product_tasks WHERE slug = $1`, slug, old); err != nil {
			return "", err
		}
		if _, err := tx.Exec(ctx, `UPDATE product_entries SET task_slug = $2 WHERE task_slug = $1`, slug, old); err != nil {
			return "", err
		}
		if _, err := tx.Exec(ctx, `UPDATE product_votes SET task_slug = $2 WHERE task_slug = $1`, slug, old); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE product_tasks SET opens_at = $2, deadline = $3 WHERE slug = $1`, slug, opens, ws.AddDate(0, 0, 7)); err != nil {
		return "", err
	}
	kind := "user"
	if actorID == "" {
		actorID, kind = "rotation", "system"
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorID: actorID, ActorKind: kind, Action: "product_task.started", AggregateKind: "product_task", AggregateID: slug,
		RequestID: httpx.RequestID(ctx)}); err != nil {
		return "", err
	}
	return slug, nil
}

// Winner is the top entry of a finished task.
type Winner struct {
	EntryID string `json:"entry_id"`
	Handle  string `json:"handle"`
	Votes   int    `json:"votes"`
	Passed  int    `json:"passed"`
	Total   int    `json:"total"`
}

func winnerOf(ctx context.Context, tx pgx.Tx, slug, kind string) (*Winner, error) {
	pick, order := rankRule(kind)
	var w Winner
	err := tx.QueryRow(ctx, `
		SELECT e.id, u.handle, `+votesOf+`, e.passed, e.total FROM (
			SELECT DISTINCT ON (user_id) * FROM product_entries WHERE task_slug = $1 AND status = 'done'
			ORDER BY user_id, `+pick+`) e
		JOIN users u ON u.id = e.user_id
		ORDER BY `+order+` LIMIT 1`, slug).Scan(&w.EntryID, &w.Handle, &w.Votes, &w.Passed, &w.Total)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &w, err
}
