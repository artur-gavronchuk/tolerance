package stacks

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/daily"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

// Stack is one canonical (tool, model) pair and how its users did at the daily task. A day is worth up to 100
// points (same scale as the overall leaderboard). "Solved" only exists for bugfix days; optimize days are
// measured by points alone, so the bugfix-only fields are null when the stack has no bugfix days.
type Stack struct {
	Tool              string   `json:"tool"`
	Model             string   `json:"model"`
	Label             string   `json:"label"`
	Users             int      `json:"users"`
	DaysAttempted     int      `json:"days_attempted"`
	BugfixDays        int      `json:"bugfix_days"`
	OptimizeDays      int      `json:"optimize_days"`
	DaysSolved        int      `json:"days_solved"`
	AvgPoints         float64  `json:"avg_points"`
	SolveRate         *float64 `json:"solve_rate"`
	AvgTestsShare     *float64 `json:"avg_tests_share"`
	AvgAttemptsToPass *float64 `json:"avg_attempts_to_pass"`
}

type attempt struct {
	madeWith      string
	solved        bool
	points        int
	passed, total int
}

type userDay struct {
	user     string
	day      string
	optimize bool
	attempts []attempt
}

// Overall ranks stacks over the last `days` days (0 = all time).
func (s *Service) Overall(ctx context.Context, days int) ([]Stack, error) {
	where, args := "", []any{}
	if days > 0 {
		where = " AND s.day >= $1::date"
		args = append(args, time.Now().UTC().AddDate(0, 0, -(days-1)).Format("2006-01-02"))
	}
	return s.compute(ctx, where, args)
}

// ForDay ranks stacks for one day.
func (s *Service) ForDay(ctx context.Context, day string) ([]Stack, error) {
	if _, ok := daily.ParseDay(day); !ok || day > daily.Today() {
		return nil, httpx.NotFound()
	}
	return s.compute(ctx, " AND s.day = $1::date", []any{day})
}

// The points CASE mirrors daily.Overall: bugfix = share of hidden tests passed; optimize = score relative to
// the day's best.
func (s *Service) compute(ctx context.Context, where string, args []any) ([]Stack, error) {
	var uds []*userDay
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT s.user_id, s.day::text, t.kind = 'optimize', s.made_with, s.status = 'passed', s.passed_tests, s.total_tests,
				(CASE
					WHEN s.passed_tests = 0 OR s.total_tests = 0 THEN 0
					WHEN t.kind = 'optimize' AND t.direction = 'max' AND db.best_max > 0 THEN round(100.0 * s.score / db.best_max)
					WHEN t.kind = 'optimize' AND t.direction = 'min' AND s.score > 0 AND s.passed_tests = s.total_tests THEN round(100.0 * db.best_min / s.score)
					WHEN t.kind = 'optimize' THEN 0
					ELSE round(100.0 * s.passed_tests / s.total_tests) END)::int
			FROM submissions s
			JOIN tasks t ON t.slug = s.task_slug
			LEFT JOIN (
				SELECT day, max(score) AS best_max,
					min(score) FILTER (WHERE score > 0 AND passed_tests = total_tests) AS best_min
				FROM submissions WHERE day IS NOT NULL AND score IS NOT NULL GROUP BY day) db ON db.day = s.day
			WHERE s.day IS NOT NULL AND s.status IN ('passed', 'failed')`+where+`
			ORDER BY s.user_id, s.day, s.created_at, s.id`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		var cur *userDay
		for rows.Next() {
			var u, d string
			var opt bool
			var a attempt
			if err := rows.Scan(&u, &d, &opt, &a.madeWith, &a.solved, &a.passed, &a.total, &a.points); err != nil {
				return err
			}
			if cur == nil || cur.user != u || cur.day != d {
				cur = &userDay{user: u, day: d, optimize: opt}
				uds = append(uds, cur)
			}
			cur.attempts = append(cur.attempts, a)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return group(uds), nil
}

type agg struct {
	users                  map[string]bool
	days, bugfix, optimize int
	solved                 int
	pointsSum              int
	shareSum               float64
	attemptsSum            int
}

// group attributes each user-day to the stack of the attempt that decided it (bugfix: the first full pass,
// else the best attempt; optimize: the best attempt) and aggregates.
func group(uds []*userDay) []Stack {
	byStack := map[[2]string]*agg{}
	for _, ud := range uds {
		best, firstPass := 0, -1
		for i, a := range ud.attempts {
			if a.solved && firstPass < 0 {
				firstPass = i
			}
			if a.points > ud.attempts[best].points {
				best = i
			}
		}
		if !ud.optimize && firstPass >= 0 {
			best = firstPass
		}
		b := ud.attempts[best]
		tool, model := Normalize(b.madeWith)
		k := [2]string{tool, model}
		g := byStack[k]
		if g == nil {
			g = &agg{users: map[string]bool{}}
			byStack[k] = g
		}
		g.users[ud.user] = true
		g.days++
		g.pointsSum += b.points
		if ud.optimize {
			g.optimize++
			continue
		}
		g.bugfix++
		if b.total > 0 {
			g.shareSum += float64(b.passed) / float64(b.total)
		}
		if firstPass >= 0 {
			g.solved++
			g.attemptsSum += firstPass + 1
		}
	}
	out := make([]Stack, 0, len(byStack))
	for k, g := range byStack {
		st := Stack{
			Tool: k[0], Model: k[1], Label: Label(k[0], k[1]),
			Users: len(g.users), DaysAttempted: g.days, BugfixDays: g.bugfix, OptimizeDays: g.optimize,
			DaysSolved: g.solved, AvgPoints: round(float64(g.pointsSum)/float64(g.days), 10),
		}
		if g.bugfix > 0 {
			sr := round(float64(g.solved)/float64(g.bugfix), 1000)
			sh := round(g.shareSum/float64(g.bugfix), 1000)
			st.SolveRate, st.AvgTestsShare = &sr, &sh
		}
		if g.solved > 0 {
			v := round(float64(g.attemptsSum)/float64(g.solved), 10)
			st.AvgAttemptsToPass = &v
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.AvgPoints != b.AvgPoints {
			return a.AvgPoints > b.AvgPoints
		}
		if sa, sb := val(a.SolveRate), val(b.SolveRate); sa != sb {
			return sa > sb
		}
		if a.DaysAttempted != b.DaysAttempted {
			return a.DaysAttempted > b.DaysAttempted
		}
		if a.Users != b.Users {
			return a.Users > b.Users
		}
		return a.Label < b.Label
	})
	return out
}

func val(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

// round rounds to 1/scale (scale 10 = one decimal, 1000 = three).
func round(v, scale float64) float64 { return float64(int(v*scale+0.5)) / scale }
