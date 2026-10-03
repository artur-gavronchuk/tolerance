// Package admin is the owner's read-only view of the platform: what happens in the three modes (daily task,
// weekly product tasks, tanks) and whether the machinery is healthy. Every number comes from one cheap
// read-only transaction over tables the other packages own.
package admin

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/rating"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/sanitize"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

// Point is one day of a series.
type Point struct {
	Day   string `json:"day"` // YYYY-MM-DD, UTC
	Value int    `json:"value"`
}

type Users struct {
	Total        int     `json:"total"`
	NewToday     int     `json:"new_today"`
	New7d        int     `json:"new_7d"`
	ActiveToday  int     `json:"active_today"`
	SignupSeries []Point `json:"signup_series"`
}

type Daily struct {
	TaskSlug         string         `json:"task_slug"`
	TaskKind         string         `json:"task_kind"`
	TaskTitle        string         `json:"task_title"`
	Today            map[string]int `json:"today"` // submissions today by status
	UniqueSolvers    int            `json:"unique_solvers"`
	InfraRate7d      float64        `json:"infra_rate_7d"`
	Submissions7d    int            `json:"submissions_7d"`
	MedianSeconds    *float64       `json:"median_seconds"` // created to verdict, last 7 days
	SubmissionSeries []Point        `json:"submission_series"`
	UserSeries       []Point        `json:"user_series"`
}

type Products struct {
	TaskSlug  string         `json:"task_slug"`
	TaskTitle string         `json:"task_title"`
	TaskKind  string         `json:"task_kind"`
	Opens     *time.Time     `json:"opens_at"`
	Deadline  *time.Time     `json:"deadline"`
	Entries   int            `json:"entries"`
	ByStatus  map[string]int `json:"by_status"`
	Votes     int            `json:"votes"`
	NextKind  string         `json:"next_kind"`
	Upcoming  int            `json:"upcoming"`
}

type LadderRow struct {
	Rank    int    `json:"rank"`
	BotID   string `json:"bot_id"`
	Name    string `json:"name"`
	Owner   string `json:"owner"`
	Rating  int    `json:"rating"`
	Matches int    `json:"matches"`
}

type Tournament struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Tanks struct {
	BotsTotal       int            `json:"bots_total"`
	BotsActive      int            `json:"bots_active"`
	UploadsToday    int            `json:"uploads_today"`
	RejectedToday   int            `json:"rejected_today"`
	MatchesLastHour map[string]int `json:"matches_last_hour"`
	Tournament      *Tournament    `json:"tournament"`
	Ladder          []LadderRow    `json:"ladder"`
}

type JobCount struct {
	Kind  string `json:"kind"`
	State string `json:"state"`
	Count int    `json:"count"`
}

type Stuck struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Status     string `json:"status"`
	AgeMinutes int    `json:"age_minutes"`
	Href       string `json:"href"`
}

type InfraError struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	ID     string    `json:"id"`
	Reason string    `json:"reason"`
}

type Health struct {
	Jobs            []JobCount   `json:"jobs"`
	OldestQueuedSec *int         `json:"oldest_queued_seconds"`
	RetriedJobs     int          `json:"retried_jobs"`
	FailedJobs      int          `json:"failed_jobs"`
	Stuck           []Stuck      `json:"stuck"`
	InfraErrors     []InfraError `json:"infra_errors"`
}

type Pulse struct {
	GeneratedAt time.Time `json:"generated_at"`
	Users       Users     `json:"users"`
	Daily       Daily     `json:"daily"`
	Products    Products  `json:"products"`
	Tanks       Tanks     `json:"tanks"`
	Health      Health    `json:"health"`
}

const days = 14

func dayStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// series runs q (which must return a UTC date and a count, with $1 the first day of the window) and fills the
// gaps so the result always has `days` points ending today.
func series(ctx context.Context, tx pgx.Tx, since time.Time, q string) ([]Point, error) {
	rows, err := tx.Query(ctx, q, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var d time.Time
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		got[d.Format("2006-01-02")] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Point, days)
	for i := range out {
		d := since.AddDate(0, 0, i).Format("2006-01-02")
		out[i] = Point{Day: d, Value: got[d]}
	}
	return out, nil
}

// counts runs a "key, count" query into a map.
func counts(ctx context.Context, tx pgx.Tx, q string, args ...any) (map[string]int, error) {
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// optional runs a single-row query that may legitimately find nothing.
func optional(ctx context.Context, tx pgx.Tx, q string, args []any, dst ...any) error {
	err := tx.QueryRow(ctx, q, args...).Scan(dst...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (s *Service) Pulse(ctx context.Context) (Pulse, error) {
	now := time.Now().UTC()
	today := dayStart(now)
	since := today.AddDate(0, 0, -(days - 1))
	d7 := now.AddDate(0, 0, -7)
	p := Pulse{GeneratedAt: now}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		// one runs a single-row query into dst; after the first failure the rest are skipped.
		one := func(q string, dst any, args ...any) {
			if err == nil {
				err = tx.QueryRow(ctx, q, args...).Scan(dst)
			}
		}

		// Users.
		u := &p.Users
		one(`SELECT count(*) FROM users`, &u.Total)
		one(`SELECT count(*) FROM users WHERE created_at >= $1`, &u.NewToday, today)
		one(`SELECT count(*) FROM users WHERE created_at >= $1`, &u.New7d, d7)
		one(`SELECT count(*) FROM (
			SELECT user_id FROM submissions WHERE created_at >= $1
			UNION SELECT user_id FROM product_entries WHERE created_at >= $1
			UNION SELECT b.owner_user_id FROM bot_versions v JOIN game_bots b ON b.id = v.bot_id
				WHERE v.source = 'upload' AND v.created_at >= $1 AND b.owner_user_id IS NOT NULL) a`, &u.ActiveToday, today)
		if err != nil {
			return err
		}
		if u.SignupSeries, err = series(ctx, tx, since, `SELECT (created_at AT TIME ZONE 'UTC')::date, count(*)::int FROM users
			WHERE created_at >= $1 GROUP BY 1`); err != nil {
			return err
		}

		// Daily task.
		d := &p.Daily
		if err = optional(ctx, tx, `SELECT t.slug, t.kind, t.title FROM daily_tasks d JOIN tasks t ON t.slug = d.task_slug WHERE d.day = $1::date`,
			[]any{today}, &d.TaskSlug, &d.TaskKind, &d.TaskTitle); err != nil {
			return err
		}
		if d.Today, err = counts(ctx, tx, `SELECT status, count(*)::int FROM submissions WHERE created_at >= $1 GROUP BY 1`, today); err != nil {
			return err
		}
		one(`SELECT count(DISTINCT user_id) FROM submissions WHERE created_at >= $1`, &d.UniqueSolvers, today)
		if err != nil {
			return err
		}
		var infra7 int
		if err = tx.QueryRow(ctx, `SELECT count(*)::int, (count(*) FILTER (WHERE status = 'infra_error'))::int FROM submissions WHERE created_at >= $1`, d7).
			Scan(&d.Submissions7d, &infra7); err != nil {
			return err
		}
		if d.Submissions7d > 0 {
			d.InfraRate7d = float64(infra7) / float64(d.Submissions7d)
		}
		if err = tx.QueryRow(ctx, `SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM finished_at - created_at))::float8
			FROM submissions WHERE finished_at IS NOT NULL AND created_at >= $1`, d7).Scan(&d.MedianSeconds); err != nil {
			return err
		}
		if d.SubmissionSeries, err = series(ctx, tx, since, `SELECT (created_at AT TIME ZONE 'UTC')::date, count(*)::int FROM submissions
			WHERE created_at >= $1 GROUP BY 1`); err != nil {
			return err
		}
		if d.UserSeries, err = series(ctx, tx, since, `SELECT (created_at AT TIME ZONE 'UTC')::date, count(DISTINCT user_id)::int FROM submissions
			WHERE created_at >= $1 GROUP BY 1`); err != nil {
			return err
		}

		// Products: the task that opened most recently (this week's, or the last one still showing).
		pr := &p.Products
		pr.ByStatus = map[string]int{}
		if err = optional(ctx, tx, `SELECT slug, title, kind, opens_at, deadline FROM product_tasks
			WHERE active AND opens_at IS NOT NULL AND opens_at <= now() ORDER BY opens_at DESC, slug LIMIT 1`, nil,
			&pr.TaskSlug, &pr.TaskTitle, &pr.TaskKind, &pr.Opens, &pr.Deadline); err != nil {
			return err
		}
		pr.Opens, pr.Deadline = utcPtr(pr.Opens), utcPtr(pr.Deadline)
		if pr.TaskSlug != "" {
			if pr.ByStatus, err = counts(ctx, tx, `SELECT status, count(*)::int FROM product_entries WHERE task_slug = $1 GROUP BY 1`, pr.TaskSlug); err != nil {
				return err
			}
			for _, n := range pr.ByStatus {
				pr.Entries += n
			}
			one(`SELECT count(*) FROM product_votes WHERE task_slug = $1`, &pr.Votes, pr.TaskSlug)
		}
		one(`SELECT count(*) FROM product_tasks WHERE active AND NOT frozen AND opens_at IS NULL`, &pr.Upcoming)
		if err != nil {
			return err
		}
		if err = optional(ctx, tx, `SELECT kind FROM product_tasks WHERE active AND NOT frozen AND opens_at IS NULL ORDER BY ord, slug LIMIT 1`, nil, &pr.NextKind); err != nil {
			return err
		}

		// Tanks.
		t := &p.Tanks
		one(`SELECT count(*) FROM game_bots WHERE NOT house`, &t.BotsTotal)
		one(`SELECT count(*) FROM game_bots WHERE NOT house AND active_version_id IS NOT NULL`, &t.BotsActive)
		one(`SELECT count(*) FROM bot_versions WHERE source = 'upload' AND created_at >= $1`, &t.UploadsToday, today)
		one(`SELECT count(*) FROM bot_versions WHERE status = 'rejected' AND created_at >= $1`, &t.RejectedToday, today)
		if err != nil {
			return err
		}
		if t.MatchesLastHour, err = counts(ctx, tx, `SELECT status, count(*)::int FROM matches WHERE created_at >= $1 GROUP BY 1`, now.Add(-time.Hour)); err != nil {
			return err
		}
		var tr Tournament
		if err = optional(ctx, tx, `SELECT id, name, status FROM tanks_tournaments WHERE status <> 'cancelled'
			ORDER BY (status = 'running') DESC, (status = 'scheduled') DESC,
			         CASE WHEN status = 'scheduled' THEN starts_at END ASC, coalesce(finished_at, starts_at) DESC LIMIT 1`, nil,
			&tr.ID, &tr.Name, &tr.Status); err != nil {
			return err
		}
		if tr.ID != "" {
			t.Tournament = &tr
		}
		if t.Ladder, err = ladderTop(ctx, tx, now); err != nil {
			return err
		}

		// Health.
		h := &p.Health
		if h.Jobs, err = jobCounts(ctx, tx); err != nil {
			return err
		}
		var oldest *float64
		if err = tx.QueryRow(ctx, `SELECT extract(epoch FROM now() - min(run_after))::float8 FROM jobs WHERE state = 'queued'`).Scan(&oldest); err != nil {
			return err
		}
		if oldest != nil {
			v := int(*oldest)
			if v < 0 {
				v = 0
			}
			h.OldestQueuedSec = &v
		}
		one(`SELECT count(*) FROM jobs WHERE attempts > 1 AND created_at >= $1`, &h.RetriedJobs, d7)
		one(`SELECT count(*) FROM jobs WHERE state = 'failed' AND created_at >= $1`, &h.FailedJobs, d7)
		if err != nil {
			return err
		}
		if h.Stuck, err = stuck(ctx, tx, now); err != nil {
			return err
		}
		h.InfraErrors, err = infraErrors(ctx, tx)
		return err
	})
	return p, err
}

// ladderTop is the five best bots of the current season by displayed rating.
func ladderTop(ctx context.Context, tx pgx.Tx, now time.Time) ([]LadderRow, error) {
	season := now.UTC().Format("2006-01")
	rows, err := tx.Query(ctx, `SELECT g.id, g.name, coalesce(u.handle, ''), coalesce(sr.mu, $2::float8), coalesce(sr.sigma, $3::float8), coalesce(sr.matches, 0)
		FROM game_bots g
		JOIN bot_versions v ON v.id = g.active_version_id
		LEFT JOIN tanks_season_ratings sr ON sr.season_id = $1 AND sr.bot_id = g.id
		LEFT JOIN users u ON u.id = g.owner_user_id
		WHERE g.id <> 'bot_house_idle'`, season, rating.DefaultMu, rating.DefaultSigma)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LadderRow{}
	for rows.Next() {
		var r LadderRow
		var mu, sigma float64
		if err := rows.Scan(&r.BotID, &r.Name, &r.Owner, &mu, &sigma, &r.Matches); err != nil {
			return nil, err
		}
		r.Rating = rating.Display(rating.Rating{Mu: mu, Sigma: sigma})
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rating != out[j].Rating {
			return out[i].Rating > out[j].Rating
		}
		if out[i].Matches != out[j].Matches {
			return out[i].Matches > out[j].Matches
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > 5 {
		out = out[:5]
	}
	for i := range out {
		out[i].Rank = i + 1
	}
	return out, nil
}

func jobCounts(ctx context.Context, tx pgx.Tx) ([]JobCount, error) {
	rows, err := tx.Query(ctx, `SELECT kind, state, count(*)::int FROM jobs
		WHERE state <> 'done' OR created_at > now() - interval '1 day' GROUP BY 1, 2 ORDER BY 1, 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JobCount{}
	for rows.Next() {
		var j JobCount
		if err := rows.Scan(&j.Kind, &j.State, &j.Count); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// stuck lists work that has been running for more than 15 minutes: it is probably not coming back on its own.
func stuck(ctx context.Context, tx pgx.Tx, now time.Time) ([]Stuck, error) {
	cut := now.Add(-15 * time.Minute)
	rows, err := tx.Query(ctx, `
		SELECT kind, id, status, since, href FROM (
			SELECT 'submission' AS kind, s.id AS id, s.status AS status, s.created_at AS since, '/u/' || u.handle AS href
				FROM submissions s JOIN users u ON u.id = s.user_id WHERE s.status = 'running' AND s.created_at < $1
			UNION ALL
			SELECT 'product entry', e.id, e.status, e.created_at, '/products/' || e.task_slug
				FROM product_entries e WHERE e.status = 'running' AND e.created_at < $1
			UNION ALL
			SELECT 'match', m.id, m.status, coalesce(m.started_at, m.created_at), '/tanks/matches/' || m.id
				FROM matches m WHERE m.status = 'running' AND coalesce(m.started_at, m.created_at) < $1
		) x ORDER BY since LIMIT 50`, cut)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Stuck{}
	for rows.Next() {
		var s Stuck
		var since time.Time
		if err := rows.Scan(&s.Kind, &s.ID, &s.Status, &since, &s.Href); err != nil {
			return nil, err
		}
		s.AgeMinutes = int(now.Sub(since).Minutes())
		out = append(out, s)
	}
	return out, rows.Err()
}

func infraErrors(ctx context.Context, tx pgx.Tx) ([]InfraError, error) {
	rows, err := tx.Query(ctx, `
		SELECT at, kind, id, reason FROM (
			(SELECT coalesce(finished_at, created_at) AS at, 'submission' AS kind, id, coalesce(failure_reason, '') AS reason
				FROM submissions WHERE status = 'infra_error' ORDER BY 1 DESC LIMIT 20)
			UNION ALL
			(SELECT coalesce(finished_at, created_at), 'product entry', id, coalesce(failure_reason, '')
				FROM product_entries WHERE status = 'infra_error' ORDER BY 1 DESC LIMIT 20)
			UNION ALL
			(SELECT coalesce(finished_at, created_at), 'match', id, failure_reason
				FROM matches WHERE status = 'infra_error' ORDER BY 1 DESC LIMIT 20)
		) x ORDER BY at DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InfraError{}
	for rows.Next() {
		var e InfraError
		if err := rows.Scan(&e.At, &e.Kind, &e.ID, &e.Reason); err != nil {
			return nil, err
		}
		e.At = e.At.UTC()
		e.Reason = sanitize.CleanText(e.Reason, 300)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Event is one line of the recent feed.
type Event struct {
	At     time.Time `json:"at"`
	Type   string    `json:"type"` // signup | submission | product_entry | bot_version | tournament
	Title  string    `json:"title"`
	Detail string    `json:"detail"`
	Status string    `json:"status"`
	Href   string    `json:"href"`
}

const feedPer = 50

// Recent merges the latest signups, submissions, product entries, bot versions and tournament results.
func (s *Service) Recent(ctx context.Context) ([]Event, error) {
	out := []Event{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT at, type, title, detail, status, href FROM (
				(SELECT created_at AS at, 'signup' AS type, handle AS title, 'signed up' AS detail, ''::text AS status, '/u/' || handle AS href
					FROM users ORDER BY created_at DESC LIMIT $1)
				UNION ALL
				(SELECT s.created_at, 'submission', u.handle, s.task_slug || ' - ' || s.passed_tests || '/' || s.total_tests || ' tests'
						|| CASE WHEN s.score IS NOT NULL THEN ', score ' || round(s.score::numeric, 2) ELSE '' END,
						s.status, CASE WHEN s.day IS NOT NULL THEN '/day/' || s.day::text ELSE '/u/' || u.handle END
					FROM submissions s JOIN users u ON u.id = s.user_id ORDER BY s.created_at DESC LIMIT $1)
				UNION ALL
				(SELECT e.created_at, 'product_entry', u.handle, e.task_slug || ' - ' || e.passed || '/' || e.total || ' scenarios',
						e.status, '/products/' || e.task_slug
					FROM product_entries e JOIN users u ON u.id = e.user_id ORDER BY e.created_at DESC LIMIT $1)
				UNION ALL
				(SELECT v.created_at, 'bot_version', g.name, 'v' || v.number || ' (' || v.source || ')', v.status, '/tanks/bots/' || g.id
					FROM bot_versions v JOIN game_bots g ON g.id = v.bot_id WHERE NOT g.house ORDER BY v.created_at DESC LIMIT $1)
				UNION ALL
				(SELECT t.finished_at, 'tournament', t.name, 'champion: ' || coalesce(g.name, 'none'), t.status, '/tanks/tournaments/' || t.id
					FROM tanks_tournaments t LEFT JOIN game_bots g ON g.id = t.champion_bot_id
					WHERE t.status = 'finished' AND t.finished_at IS NOT NULL ORDER BY t.finished_at DESC LIMIT $1)
			) x ORDER BY at DESC LIMIT $1`, feedPer)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e Event
			if err := rows.Scan(&e.At, &e.Type, &e.Title, &e.Detail, &e.Status, &e.Href); err != nil {
				return err
			}
			e.At = e.At.UTC()
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}
