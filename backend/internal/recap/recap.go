// Package recap is the signed-in person's "yesterday" card and their last seven days of the daily task. It only
// reads: the ranking comes from daily.Leaderboard, so a place here is the place on the day's board.
package recap

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/daily"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
)

type Service struct {
	pool  *db.Pool
	daily *daily.Service
}

func NewService(pool *db.Pool, d *daily.Service) *Service { return &Service{pool: pool, daily: d} }

type Task struct {
	Slug      string  `json:"slug"`
	Title     string  `json:"title"`
	Kind      string  `json:"kind"`
	Direction *string `json:"direction"`
}

// Result is a person's (or the winner's) best result of a day.
type Result struct {
	Handle      string   `json:"handle,omitempty"`
	PassedTests int      `json:"passed_tests"`
	TotalTests  int      `json:"total_tests"`
	Score       *float64 `json:"score"`
}

// Yesterday is the closed day before today. Mine is nil when the person did not submit; Place is nil when they
// are not on the day's board; Winner is nil when nobody ranked.
type Yesterday struct {
	Day          string  `json:"day"`
	Task         Task    `json:"task"`
	Mine         *Result `json:"mine"`
	Place        *int    `json:"place"`
	Participants int     `json:"participants"` // people on the board
	Winner       *Result `json:"winner"`
}

type Response struct {
	Yesterday  *Yesterday   `json:"yesterday"`
	Streak     daily.Streak `json:"streak"`
	SolvedDays []string     `json:"solved_days"` // of the last 7 days, today included
	Today      string       `json:"today"`
}

func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/me/recap", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.Get(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, out)
	})
}

func (s *Service) Get(ctx context.Context, userID string) (Response, error) {
	out := Response{SolvedDays: []string{}, Today: daily.Today()}
	var err error
	if out.Streak, err = s.daily.StreakOf(ctx, userID); err != nil {
		return out, err
	}
	week := time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02")
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT DISTINCT day::text FROM submissions WHERE user_id = $1 AND status = 'passed' AND day >= $2::date ORDER BY 1`, userID, week)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d string
			if err := rows.Scan(&d); err != nil {
				return err
			}
			out.SolvedDays = append(out.SolvedDays, d)
		}
		return rows.Err()
	})
	if err != nil {
		return out, err
	}
	out.Yesterday, err = s.YesterdayOf(ctx, userID)
	return out, err
}

// YesterdayOf is nil when no task ran yesterday.
func (s *Service) YesterdayOf(ctx context.Context, userID string) (*Yesterday, error) {
	day := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	y := &Yesterday{Day: day}
	var handle string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT handle FROM users WHERE id = $1`, userID).Scan(&handle); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT t.slug, t.title, t.kind, t.direction FROM daily_tasks d JOIN tasks t ON t.slug = d.task_slug WHERE d.day = $1::date`, day).
			Scan(&y.Task.Slug, &y.Task.Title, &y.Task.Kind, &y.Task.Direction)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	board, err := s.daily.Leaderboard(ctx, day)
	if err != nil {
		return nil, err
	}
	y.Participants = len(board)
	if len(board) > 0 {
		w := board[0]
		y.Winner = &Result{Handle: w.Handle, PassedTests: w.PassedTests, TotalTests: w.TotalTests, Score: w.Score}
	}
	for _, r := range board {
		if r.Handle == handle {
			place := r.Place
			y.Place = &place
			y.Mine = &Result{Handle: r.Handle, PassedTests: r.PassedTests, TotalTests: r.TotalTests, Score: r.Score}
		}
	}
	if y.Mine == nil {
		// Not on the board (nothing passed, or an invalid min score): still show the person's best finished attempt.
		var m Result
		err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `
				SELECT passed_tests, total_tests, score FROM submissions
				WHERE user_id = $1 AND day = $2::date AND status IN ('passed', 'failed')
				ORDER BY passed_tests DESC, created_at LIMIT 1`, userID, day).Scan(&m.PassedTests, &m.TotalTests, &m.Score)
		})
		if err == nil {
			y.Mine = &m
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return y, nil
}
