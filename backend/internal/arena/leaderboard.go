package arena

import (
	"context"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/skillrating"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

// Leaderboard returns one skill's table, best first. The order is
// access = rating − uncertainty descending, and on equal access a rating
// confirmed on the agent's current version outranks one carried over from a
// configuration the owner has since replaced: that rating is weaker evidence,
// and the table sorts it lower instead of hiding it.
//
// Agents that opted out of the public tables (agents.public = false) and banned
// agents are absent. Their ratings keep being computed; only the display stops.
func (s *Service) Leaderboard(ctx context.Context, skill string, limit int) ([]Row, error) {
	out := []Row{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT a.name, av.number, av.model, av.harness,
				r.rating, r.uncertainty, r.runs, r.updated_at, (r.version_id = a.current_version_id) AS on_current
			FROM skill_ratings r
			JOIN agents a ON a.id = r.agent_id
			JOIN agent_versions av ON av.id = r.version_id
			WHERE r.skill_slug = $1 AND a.public AND a.banned_at IS NULL
			ORDER BY (r.rating - r.uncertainty) DESC, on_current DESC, r.rating DESC, a.name
			LIMIT $2`, skill, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Row
			if err := rows.Scan(&r.AgentName, &r.VersionNumber, &r.Model, &r.Harness,
				&r.Rating, &r.Uncertainty, &r.Runs, &r.ScoredAt, &r.OnCurrentVersion); err != nil {
				return err
			}
			r.ScoredAt = r.ScoredAt.UTC()
			r.Access = skillrating.Access(r.Rating, r.Uncertainty)
			r.Tier = skillrating.Tier(r.Access)
			r.Rank = len(out) + 1
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
