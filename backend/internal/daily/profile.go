package daily

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/httpx"
)

type ProfileDay struct {
	Day         string   `json:"day"`
	Task        DayTask  `json:"task"`
	Status      string   `json:"status"` // passed | failed: of the best attempt
	PassedTests int      `json:"passed_tests"`
	TotalTests  int      `json:"total_tests"`
	Score       *float64 `json:"score"` // optimize days only
	MadeWith    string   `json:"made_with"`
	Attempts    int      `json:"attempts"`
}

type Profile struct {
	Handle     string       `json:"handle"`
	JoinedAt   time.Time    `json:"joined_at"`
	Streak     Streak       `json:"streak"`
	SolvedDays int          `json:"solved_days"`
	PlayedDays int          `json:"played_days"`
	Place      *int         `json:"place"` // on the overall leaderboard, if in its top 100
	Tools      []string     `json:"tools"` // distinct "made with" values, most used first
	Days       []ProfileDay `json:"days"`  // newest first, up to 90
}

// ProfileOf is a person's public record on daily tasks: streak, place and their best attempt per day.
// Practice uploads (day IS NULL) are not part of it.
func (s *Service) ProfileOf(ctx context.Context, handle string) (Profile, error) {
	p := Profile{Tools: []string{}, Days: []ProfileDay{}}
	var userID string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id, handle, created_at FROM users WHERE lower(handle) = lower($1) AND banned_at IS NULL AND NOT house`, handle).
			Scan(&userID, &p.Handle, &p.JoinedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		p.JoinedAt = p.JoinedAt.UTC()
		m, err := solvedDays(ctx, tx, `AND user_id = $1`, userID)
		if err != nil {
			return err
		}
		p.SolvedDays = len(m[userID])
		p.Streak = streakOf(m[userID], time.Now().UTC().Truncate(24*time.Hour))

		rows, err := tx.Query(ctx, `
			SELECT b.day::text, t.slug, t.title, t.language, t.difficulty, b.status, b.passed_tests, b.total_tests,
			       b.score, b.made_with, b.attempts
			FROM (
				SELECT DISTINCT ON (s.day) s.day, s.task_slug, s.status, s.passed_tests, s.total_tests, s.score, s.made_with,
				       count(*) OVER (PARTITION BY s.day) AS attempts
				FROM submissions s
				WHERE s.user_id = $1 AND s.day IS NOT NULL AND s.status IN ('passed', 'failed') AND s.hidden_at IS NULL
				ORDER BY s.day, (s.status = 'passed') DESC, s.passed_tests DESC, s.score DESC NULLS LAST, s.created_at ASC) b
			JOIN tasks t ON t.slug = b.task_slug
			ORDER BY b.day DESC LIMIT 90`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d ProfileDay
			if err := rows.Scan(&d.Day, &d.Task.Slug, &d.Task.Title, &d.Task.Language, &d.Task.Difficulty, &d.Status,
				&d.PassedTests, &d.TotalTests, &d.Score, &d.MadeWith, &d.Attempts); err != nil {
				return err
			}
			p.Days = append(p.Days, d)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		p.PlayedDays = len(p.Days)

		tools, err := tx.Query(ctx, `
			SELECT min(trim(made_with)) FROM submissions
			WHERE user_id = $1 AND day IS NOT NULL AND hidden_at IS NULL AND trim(made_with) <> ''
			GROUP BY lower(trim(made_with)) ORDER BY count(*) DESC, 1 LIMIT 10`, userID)
		if err != nil {
			return err
		}
		defer tools.Close()
		for tools.Next() {
			var t string
			if err := tools.Scan(&t); err != nil {
				return err
			}
			p.Tools = append(p.Tools, t)
		}
		return tools.Err()
	})
	if err != nil {
		return Profile{}, err
	}
	overall, err := s.Overall(ctx)
	if err != nil {
		return Profile{}, err
	}
	for _, r := range overall {
		if r.Handle == p.Handle {
			place := r.Place
			p.Place = &place
			break
		}
	}
	return p, nil
}
