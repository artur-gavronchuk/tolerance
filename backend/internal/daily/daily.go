// Package daily is the task of the day: one coding task per UTC day, assigned lazily on the first request,
// plus the day's leaderboard, the archive of past days, the overall leaderboard and streaks.
package daily

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/ttlcache"
	"tolerance/internal/tasks"
)

// AttemptsPerDay is how many graded attempts a person gets at the daily task.
const AttemptsPerDay = 3

const dayLayout = "2006-01-02"

// Today is the current UTC day, YYYY-MM-DD.
func Today() string { return time.Now().UTC().Format(dayLayout) }

// ParseDay validates a YYYY-MM-DD day.
func ParseDay(s string) (time.Time, bool) {
	t, err := time.Parse(dayLayout, s)
	return t, err == nil
}

type Service struct {
	pool    *db.Pool
	overall ttlcache.Cache[struct{}, []OverallRow]

	// OnAssign, when set, is called once after a day's task is first assigned (by whichever request or tick
	// got there first). The house agents hook in here (internal/house).
	OnAssign func(ctx context.Context, day string)
	// HouseHandles are the configured house agents, so today's summary can list one that has not run yet.
	HouseHandles []string
}

func NewService(pool *db.Pool) *Service {
	return &Service{pool: pool, overall: ttlcache.Cache[struct{}, []OverallRow]{TTL: 15 * time.Second}}
}

// ErrNoTasks means the catalog has no active task to assign.
var ErrNoTasks = httpx.New(http.StatusServiceUnavailable, "no_tasks", "No task is available today")

// TaskFor returns the slug of the task assigned to day, assigning one when day is today and none is set
// yet. For any other day without a task it returns pgx.ErrNoRows.
//
// Assignment picks the active task used least recently (never-used first, then the oldest last day, ties
// by slug). INSERT ... ON CONFLICT DO NOTHING followed by a re-read makes two simultaneous first requests
// of the day converge on the same task.
func (s *Service) TaskFor(ctx context.Context, day string) (string, error) {
	var slug string
	assigned := false
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		assigned = false
		err := tx.QueryRow(ctx, `SELECT task_slug FROM daily_tasks WHERE day = $1`, day).Scan(&slug)
		if err == nil || !errors.Is(err, pgx.ErrNoRows) || day != Today() {
			return err
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO daily_tasks (day, task_slug)
			SELECT $1::date, t.slug FROM tasks t WHERE t.active
			ORDER BY (SELECT max(d.day) FROM daily_tasks d WHERE d.task_slug = t.slug) ASC NULLS FIRST, t.slug
			LIMIT 1
			ON CONFLICT (day) DO NOTHING`, day)
		if err != nil {
			return err
		}
		assigned = tag.RowsAffected() == 1
		return tx.QueryRow(ctx, `SELECT task_slug FROM daily_tasks WHERE day = $1`, day).Scan(&slug)
	})
	if errors.Is(err, pgx.ErrNoRows) && day == Today() {
		return "", ErrNoTasks
	}
	if err == nil && assigned && s.OnAssign != nil {
		s.OnAssign(ctx, day)
	}
	return slug, err
}

// Daily is the GET /daily response. My is filled by the caller-supplied MyFunc when a session is present.
type Daily struct {
	Day            string        `json:"day"`
	ClosesAt       time.Time     `json:"closes_at"`
	IsOpen         bool          `json:"is_open"`
	Task           tasks.Summary `json:"task"`
	AttemptsPerDay int           `json:"attempts_per_day"`
	My             any           `json:"my"`
	House          []HouseResult `json:"house"` // what the platform's own agents scored (house.go)
}

// MyFunc builds the signed-in person's block for a day. It lives outside this package (it needs the
// submission type) and is injected.
type MyFunc func(ctx context.Context, userID, day string) (any, error)

func (s *Service) Get(ctx context.Context, day, userID string, my MyFunc) (Daily, error) {
	t, ok := ParseDay(day)
	if !ok || day > Today() {
		return Daily{}, httpx.NotFound()
	}
	slug, err := s.TaskFor(ctx, day)
	if errors.Is(err, pgx.ErrNoRows) {
		return Daily{}, httpx.NotFound()
	}
	if err != nil {
		return Daily{}, err
	}
	task, err := tasks.Get(ctx, s.pool, slug)
	if err != nil {
		return Daily{}, err
	}
	d := Daily{Day: day, ClosesAt: t.Add(24 * time.Hour).UTC(), IsOpen: day == Today(), Task: task, AttemptsPerDay: AttemptsPerDay}
	if d.House, err = s.houseResults(ctx, day, task, userID); err != nil {
		return Daily{}, err
	}
	if userID != "" && my != nil {
		if d.My, err = my(ctx, userID, day); err != nil {
			return Daily{}, err
		}
	}
	return d, nil
}

type Row struct {
	ID          string    `json:"id"` // the submission, for moderation
	Place       int       `json:"place"`
	Handle      string    `json:"handle"`
	MadeWith    string    `json:"made_with"`
	PassedTests int       `json:"passed_tests"`
	TotalTests  int       `json:"total_tests"`
	Score       *float64  `json:"score"`
	SubmittedAt time.Time `json:"submitted_at"`
	House       bool      `json:"house"`      // a platform agent: shown inline, takes no place
	HouseName   string    `json:"house_name"` // display name of a house agent
}

// Leaderboard is the day's best finished submission per person with at least one hidden test passed (house
// agents are listed even at zero, so the board is never empty while they ran). House
// agents' rows sit inline in rank order but take no place (Place is 0): places count people only.
func (s *Service) Leaderboard(ctx context.Context, day string) ([]Row, error) {
	if _, ok := ParseDay(day); !ok || day > Today() {
		return nil, httpx.NotFound()
	}
	out := []Row{}
	people := 0
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var kind string
		var direction *string
		err := tx.QueryRow(ctx, `SELECT t.kind, t.direction FROM daily_tasks d JOIN tasks t ON t.slug = d.task_slug WHERE d.day = $1::date`, day).Scan(&kind, &direction)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		q := `
			SELECT id, handle, made_with, passed_tests, total_tests, NULL::float8, created_at, house, house_name FROM (
				SELECT DISTINCT ON (s.user_id) s.id, u.handle, u.house, u.house_name, s.made_with, s.passed_tests, s.total_tests, s.created_at
				FROM submissions s JOIN users u ON u.id = s.user_id
				WHERE s.day = $1 AND s.status IN ('passed', 'failed') AND (s.passed_tests > 0 OR u.house) AND s.hidden_at IS NULL
				ORDER BY s.user_id, s.passed_tests DESC, s.created_at ASC) best
			ORDER BY passed_tests DESC, created_at ASC, handle LIMIT 200`
		if kind == "optimize" {
			// Best score per person by direction, ties by the earlier submission. A failed submission still
			// ranks (the score is the sum of its valid cases), except on a min task, where an invalid case
			// scores 0 and would be the best possible: there only fully valid submissions rank.
			dir := "DESC"
			if direction != nil && *direction == "min" {
				dir = "ASC"
			}
			q = `
			SELECT id, handle, made_with, passed_tests, total_tests, score, created_at, house, house_name FROM (
				SELECT DISTINCT ON (s.user_id) s.id, u.handle, u.house, u.house_name, s.made_with, s.passed_tests, s.total_tests, s.score, s.created_at
				FROM submissions s JOIN users u ON u.id = s.user_id JOIN tasks t ON t.slug = s.task_slug
				WHERE s.day = $1 AND s.status IN ('passed', 'failed') AND s.score IS NOT NULL AND s.hidden_at IS NULL
				  AND (t.direction <> 'min' OR s.cases_valid = t.cases)
				ORDER BY s.user_id, s.score ` + dir + `, s.created_at ASC) best
			ORDER BY score ` + dir + `, created_at ASC, handle LIMIT 200`
		}
		rows, err := tx.Query(ctx, q, day)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Row
			if err := rows.Scan(&r.ID, &r.Handle, &r.MadeWith, &r.PassedTests, &r.TotalTests, &r.Score, &r.SubmittedAt, &r.House, &r.HouseName); err != nil {
				return err
			}
			r.SubmittedAt = r.SubmittedAt.UTC()
			if !r.House {
				people++
				r.Place = people
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

type DayTask struct {
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Language   string `json:"language"`
	Difficulty int    `json:"difficulty"`
}

type DayItem struct {
	Day     string  `json:"day"`
	Task    DayTask `json:"task"`
	Solvers int     `json:"solvers"`
}

// Days lists past days, newest first, up to 60. solvers = people with a fully passed submission that day.
func (s *Service) Days(ctx context.Context) ([]DayItem, error) {
	out := []DayItem{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT d.day::text, t.slug, t.title, t.language, t.difficulty,
			       (SELECT count(DISTINCT s.user_id) FROM submissions s JOIN users u ON u.id = s.user_id AND NOT u.house WHERE s.day = d.day AND s.status = 'passed' AND s.hidden_at IS NULL)
			FROM daily_tasks d JOIN tasks t ON t.slug = d.task_slug
			WHERE d.day < $1::date ORDER BY d.day DESC LIMIT 60`, Today())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var it DayItem
			if err := rows.Scan(&it.Day, &it.Task.Slug, &it.Task.Title, &it.Task.Language, &it.Task.Difficulty, &it.Solvers); err != nil {
				return err
			}
			out = append(out, it)
		}
		return rows.Err()
	})
	return out, err
}

type Streak struct {
	Current int `json:"current"`
	Best    int `json:"best"`
}

// streakOf computes the current and best runs from the set of solved days (YYYY-MM-DD). The current run
// is the consecutive days ending today, or yesterday when today is not solved yet.
func streakOf(days []string, today time.Time) Streak {
	set := make(map[string]bool, len(days))
	for _, d := range days {
		set[d] = true
	}
	sorted := append([]string(nil), days...)
	sort.Strings(sorted)
	var st Streak
	run := 0
	var prev time.Time
	for _, d := range sorted {
		t, _ := ParseDay(d)
		if run > 0 && t.Equal(prev.AddDate(0, 0, 1)) {
			run++
		} else {
			run = 1
		}
		prev = t
		if run > st.Best {
			st.Best = run
		}
	}
	cur := today
	if !set[cur.Format(dayLayout)] {
		cur = cur.AddDate(0, 0, -1)
	}
	for set[cur.Format(dayLayout)] {
		st.Current++
		cur = cur.AddDate(0, 0, -1)
	}
	return st
}

func solvedDays(ctx context.Context, tx pgx.Tx, where string, args ...any) (map[string][]string, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT user_id, day::text FROM submissions WHERE day IS NOT NULL AND status = 'passed' AND hidden_at IS NULL
		AND user_id NOT IN (SELECT id FROM users WHERE house) `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var u, d string
		if err := rows.Scan(&u, &d); err != nil {
			return nil, err
		}
		out[u] = append(out[u], d)
	}
	return out, rows.Err()
}

// StreakOf is the signed-in person's streak.
func (s *Service) StreakOf(ctx context.Context, userID string) (Streak, error) {
	var st Streak
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		m, err := solvedDays(ctx, tx, `AND user_id = $1`, userID)
		if err != nil {
			return err
		}
		st = streakOf(m[userID], time.Now().UTC().Truncate(24*time.Hour))
		return nil
	})
	return st, err
}

type OverallRow struct {
	Place         int    `json:"place"`
	Handle        string `json:"handle"`
	Points        int    `json:"points"`
	SolvedDays    int    `json:"solved_days"`
	CurrentStreak int    `json:"current_streak"`
}

// Overall ranks people by points, then days fully solved, then current streak, then handle. A day is worth
// up to 100 points: for a bugfix day the share of hidden tests the person's best attempt passed, for an
// optimize day the best score relative to the day's best (the leader gets 100).
//
// The ranking reads every graded submission, so it is kept for 15 seconds (the profile page asks for it too).
func (s *Service) Overall(ctx context.Context) ([]OverallRow, error) {
	return s.overall.Get(struct{}{}, func() ([]OverallRow, error) { return s.computeOverall(ctx) })
}

func (s *Service) computeOverall(ctx context.Context) ([]OverallRow, error) {
	out := []OverallRow{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		m, err := solvedDays(ctx, tx, ``)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT u.handle, u.id, sum(best.points)::int FROM (
				SELECT user_id, max(CASE
					WHEN t.kind = 'optimize' AND t.direction = 'max' AND db.best_max > 0 THEN round(100.0 * s.score / db.best_max)
					WHEN t.kind = 'optimize' AND t.direction = 'min' AND s.score > 0 AND s.passed_tests = s.total_tests THEN round(100.0 * db.best_min / s.score)
					WHEN t.kind = 'optimize' THEN 0
					ELSE round(100.0 * s.passed_tests / s.total_tests) END) AS points
				FROM submissions s
				JOIN tasks t ON t.slug = s.task_slug
				LEFT JOIN (
					SELECT day, max(score) AS best_max,
						min(score) FILTER (WHERE score > 0 AND passed_tests = total_tests) AS best_min
					FROM submissions WHERE day IS NOT NULL AND score IS NOT NULL AND hidden_at IS NULL
					  AND user_id NOT IN (SELECT id FROM users WHERE house) GROUP BY day) db ON db.day = s.day
				WHERE s.day IS NOT NULL AND s.status IN ('passed', 'failed') AND s.passed_tests > 0 AND s.total_tests > 0 AND s.hidden_at IS NULL
				GROUP BY s.user_id, s.day) best
			JOIN users u ON u.id = best.user_id AND NOT u.house
			GROUP BY u.id, u.handle`)
		if err != nil {
			return err
		}
		defer rows.Close()
		today := time.Now().UTC().Truncate(24 * time.Hour)
		for rows.Next() {
			var r OverallRow
			var id string
			if err := rows.Scan(&r.Handle, &id, &r.Points); err != nil {
				return err
			}
			r.SolvedDays = len(m[id])
			r.CurrentStreak = streakOf(m[id], today).Current
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		if a.SolvedDays != b.SolvedDays {
			return a.SolvedDays > b.SolvedDays
		}
		if a.CurrentStreak != b.CurrentStreak {
			return a.CurrentStreak > b.CurrentStreak
		}
		return a.Handle < b.Handle
	})
	if len(out) > 100 {
		out = out[:100]
	}
	for i := range out {
		out[i].Place = i + 1
	}
	return out, nil
}

type RevealFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type Solution struct {
	ID          string    `json:"id"`
	Place       int       `json:"place"`
	Handle      string    `json:"handle"`
	MadeWith    string    `json:"made_with"`
	SubmittedAt time.Time `json:"submitted_at"`
	Diff        string    `json:"diff"`
}

// Reveal is what a day publishes once it closes: the hidden tests and the earliest fully passing
// solutions.
type Reveal struct {
	HiddenTests []RevealFile `json:"hidden_tests"`
	Solutions   []Solution   `json:"solutions"`
}

// ErrNotClosed means the day is still open and nothing is published yet.
var ErrNotClosed = httpx.New(http.StatusForbidden, "day_open", "Hidden tests and solutions are published when the day closes")

// Reveal returns a closed day's hidden tests and up to 10 solutions: each person's first fully passing
// submission made while the day was open, earliest first.
func (s *Service) Reveal(ctx context.Context, day string) (Reveal, error) {
	if _, ok := ParseDay(day); !ok || day > Today() {
		return Reveal{}, httpx.NotFound()
	}
	if day == Today() {
		return Reveal{}, ErrNotClosed
	}
	out := Reveal{HiddenTests: []RevealFile{}, Solutions: []Solution{}}
	var hiddenTar []byte
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT t.hidden_tar FROM daily_tasks d JOIN tasks t ON t.slug = d.task_slug WHERE d.day = $1`, day).
			Scan(&hiddenTar); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, handle, made_with, created_at, diff FROM (
				SELECT DISTINCT ON (s.user_id) s.id, u.handle, s.made_with, s.created_at, s.diff
				FROM submissions s JOIN users u ON u.id = s.user_id AND NOT u.house
				WHERE s.day = $1 AND s.status = 'passed' AND s.hidden_at IS NULL
				ORDER BY s.user_id, s.created_at ASC) first
			ORDER BY created_at ASC, handle LIMIT 10`, day)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sol Solution
			if err := rows.Scan(&sol.ID, &sol.Handle, &sol.MadeWith, &sol.SubmittedAt, &sol.Diff); err != nil {
				return err
			}
			sol.SubmittedAt = sol.SubmittedAt.UTC()
			sol.Place = len(out.Solutions) + 1
			out.Solutions = append(out.Solutions, sol)
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Reveal{}, httpx.NotFound()
	}
	if err != nil {
		return Reveal{}, err
	}
	files, err := tasks.ReadTar(hiddenTar)
	if err != nil {
		return Reveal{}, err
	}
	for p, body := range files {
		out.HiddenTests = append(out.HiddenTests, RevealFile{Path: p, Content: string(body)})
	}
	sort.Slice(out.HiddenTests, func(i, j int) bool { return out.HiddenTests[i].Path < out.HiddenTests[j].Path })
	return out, nil
}

type ToolStat struct {
	MadeWith     string `json:"made_with"` // "" = not stated
	Participants int    `json:"participants"`
	Solvers      int    `json:"solvers"`
}

type Stats struct {
	Participants int        `json:"participants"`
	Solvers      int        `json:"solvers"`
	Submissions  int        `json:"submissions"`
	ByTool       []ToolStat `json:"by_tool"`
}

// Stats summarises a day's finished submissions: how many people tried and
// solved it, overall and per "made with" tool (compared case-insensitively,
// each person counted under the tool of their best submission).
func (s *Service) Stats(ctx context.Context, day string) (Stats, error) {
	if _, ok := ParseDay(day); !ok || day > Today() {
		return Stats{}, httpx.NotFound()
	}
	st := Stats{ByTool: []ToolStat{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT count(*), count(DISTINCT user_id), count(DISTINCT user_id) FILTER (WHERE status = 'passed')
			FROM submissions WHERE day = $1 AND status IN ('passed', 'failed') AND hidden_at IS NULL
			  AND user_id NOT IN (SELECT id FROM users WHERE house)`, day).
			Scan(&st.Submissions, &st.Participants, &st.Solvers); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT tool, count(*), count(*) FILTER (WHERE solved) FROM (
				SELECT DISTINCT ON (user_id) lower(trim(made_with)) AS tool, status = 'passed' AS solved
				FROM submissions WHERE day = $1 AND status IN ('passed', 'failed') AND hidden_at IS NULL
				  AND user_id NOT IN (SELECT id FROM users WHERE house)
				ORDER BY user_id, passed_tests DESC, created_at ASC) best
			GROUP BY tool ORDER BY count(*) FILTER (WHERE solved) DESC, count(*) DESC, tool LIMIT 20`, day)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t ToolStat
			if err := rows.Scan(&t.MadeWith, &t.Participants, &t.Solvers); err != nil {
				return err
			}
			st.ByTool = append(st.ByTool, t)
		}
		return rows.Err()
	})
	return st, err
}
