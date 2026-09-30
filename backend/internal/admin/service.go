// Package admin is the platform operator's API: taking a leaked task out of the
// pool, voiding a run, and banning an agent. There is no operator UI in this
// slice — these routes are called by hand — but every action is audited with the
// reason its caller gave, because each of them changes what the public sees.
package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/skillrating"
	"tolerance/internal/skills"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

// requireReason refuses a blank reason. Every operation here is recorded and
// read back later by someone asking why the platform did this to an owner, and
// "" is not an answer.
func requireReason(reason string) (string, error) {
	r := strings.TrimSpace(reason)
	if r == "" {
		return "", httpx.WithField(http.StatusUnprocessableEntity, "validation_failed", "reason is required", "reason", "required")
	}
	return r, nil
}

// VoidRun cancels a scored qualification run and takes its evidence back out of
// the agent's rating. It is the only operation on the platform that changes a
// rating after the fact — which is why it is manual, needs a reason, and is
// audited. When the run's evidence is no longer part of the stored rating
// (the owner has since changed configuration, so the rating belongs to a newer
// version) the run is still marked, but the rating is left exactly as it is.
func (s *Service) VoidRun(ctx context.Context, actorID, runID, reason string) error {
	reason, err := requireReason(reason)
	if err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var agentID, versionID, skill, status string
		var score *float64
		err := tx.QueryRow(ctx, `SELECT agent_id, version_id, skill_slug, status, score::float8
			FROM qualification_runs WHERE id = $1 FOR UPDATE`, runID).Scan(&agentID, &versionID, &skill, &status, &score)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		switch {
		case status == "voided":
			return httpx.New(http.StatusConflict, "already_voided", "This run is already voided")
		case status != "scored" || score == nil:
			return httpx.New(http.StatusConflict, "run_not_scored", "Only a scored run can be voided")
		}

		var st skillrating.State
		var ratingVersion string
		err = tx.QueryRow(ctx, `SELECT version_id, rating, uncertainty, runs, sum_targets, prior_rating
			FROM skill_ratings WHERE agent_id = $1 AND skill_slug = $2 FOR UPDATE`, agentID, skill).
			Scan(&ratingVersion, &st.Rating, &st.Uncertainty, &st.Runs, &st.SumTargets, &st.Prior)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// Nothing stored to correct; marking the run is all that is left to do.
		case err != nil:
			return err
		case ratingVersion != versionID:
			// The rating has moved on to a newer version, so this run is not part
			// of it any more. Touching it would subtract evidence that was never
			// added.
		default:
			next := skillrating.Remove(st, skillrating.Target(*score))
			if skillrating.Unrated(next) {
				if _, err := tx.Exec(ctx, `DELETE FROM skill_ratings WHERE agent_id = $1 AND skill_slug = $2`, agentID, skill); err != nil {
					return err
				}
			} else if _, err := tx.Exec(ctx, `UPDATE skill_ratings SET rating = $3, uncertainty = $4, runs = $5,
				sum_targets = $6, updated_at = now() WHERE agent_id = $1 AND skill_slug = $2`,
				agentID, skill, next.Rating, next.Uncertainty, next.Runs, next.SumTargets); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE qualification_runs SET status = 'voided', voided_at = now(), voided_reason = $2
			WHERE id = $1`, runID, reason); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, ActorKind: identity.KindUser, Action: "admin.run_voided",
			AggregateKind: "qualification_run", AggregateID: runID, Reason: reason,
			Payload: map[string]any{"agent_id": agentID, "skill": skill}, RequestID: httpx.RequestID(ctx)})
	})
}

// RetireTask takes a task out of the pool for a leak the exposure counter cannot
// see. Ratings already earned on it are never recalculated.
func (s *Service) RetireTask(ctx context.Context, actorID, slug, reason string) error {
	reason, err := requireReason(reason)
	if err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := skills.Retire(ctx, tx, slug, reason); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, ActorKind: identity.KindUser, Action: "admin.task_retired",
			AggregateKind: "skill_task", AggregateID: slug, Reason: reason, RequestID: httpx.RequestID(ctx)})
	})
}

// TaskStats is one skill's catalog with exposure and observed difficulty, for
// deciding what to retire and what to re-weight.
func (s *Service) TaskStats(ctx context.Context, skill string) ([]skills.TaskStat, error) {
	var out []skills.TaskStat
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = skills.TaskStats(ctx, tx, skill)
		return err
	})
	return out, err
}

func (s *Service) BanAgent(ctx context.Context, actorID, agentID, reason string) error {
	return s.setBan(ctx, actorID, agentID, reason, true)
}

func (s *Service) UnbanAgent(ctx context.Context, actorID, agentID, reason string) error {
	return s.setBan(ctx, actorID, agentID, reason, false)
}

// setBan hides an agent from every public surface, or puts it back. A ban stops
// the agent competing; it does not delete anything it earned, so lifting one
// restores the same numbers.
func (s *Service) setBan(ctx context.Context, actorID, agentID, reason string, ban bool) error {
	reason, err := requireReason(reason)
	if err != nil {
		return err
	}
	action := "admin.agent_unbanned"
	if ban {
		action = "admin.agent_banned"
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `UPDATE agents SET banned_at = CASE WHEN $2 THEN coalesce(banned_at, now()) ELSE NULL END,
			banned_reason = CASE WHEN $2 THEN $3 ELSE '' END WHERE id = $1`, agentID, ban, reason)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return httpx.NotFound()
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, ActorKind: identity.KindUser, Action: action,
			AggregateKind: "agent", AggregateID: agentID, Reason: reason, RequestID: httpx.RequestID(ctx)})
	})
}
