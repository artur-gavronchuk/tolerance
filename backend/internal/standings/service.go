package standings

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

const cols = `rank, name, author, points, wins, competition_wins, match_wins, submissions, avg`

func scan(row interface{ Scan(...any) error }, s *Standing) error {
	return row.Scan(&s.Rank, &s.Agent, &s.Author, &s.Points, &s.Wins, &s.CompetitionWins, &s.MatchWins, &s.Submissions, &s.Avg)
}

func (s *Service) ForAgent(ctx context.Context, agentID string) (Standing, error) {
	var st Standing
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM agent_standings WHERE agent_id = $1`, agentID), &st)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Standing{}, httpx.NotFound()
	}
	return st, err
}

func (s *Service) Leaderboard(ctx context.Context, limit int) ([]Standing, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	out := []Standing{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+cols+` FROM agent_standings WHERE rank IS NOT NULL ORDER BY rank LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var st Standing
			if err := scan(rows, &st); err != nil {
				return err
			}
			out = append(out, st)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) BadgesForAgent(ctx context.Context, agentID string) ([]Badge, error) {
	out := []Badge{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT code, awarded_at FROM agent_badges WHERE agent_id = $1 ORDER BY awarded_at`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b Badge
			if err := rows.Scan(&b.Code, &b.AwardedAt); err != nil {
				return err
			}
			// pgx decodes timestamptz into time.Local; normalize to UTC.
			b.AwardedAt = b.AwardedAt.UTC()
			info := Catalog[b.Code]
			b.Label, b.Description = info.Label, info.Description
			out = append(out, b)
		}
		return rows.Err()
	})
	return out, err
}
