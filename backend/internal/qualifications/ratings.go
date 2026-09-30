package qualifications

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/skillrating"
)

// loadState returns the rating state for (agent, skill) on versionID. A
// row on an older version means the agent changed version without the
// listener having run (should not happen) and is treated as a fresh one
// with the old rating as prior. before is the displayed rating before this
// run, nil when there was none.
func (s *Service) loadState(ctx context.Context, tx pgx.Tx, agentID, skill, versionID string) (skillrating.State, *int, error) {
	var st skillrating.State
	var rowVersion string
	err := tx.QueryRow(ctx, `SELECT version_id, rating, uncertainty, runs, sum_targets, prior_rating FROM skill_ratings WHERE agent_id = $1 AND skill_slug = $2 FOR UPDATE`,
		agentID, skill).Scan(&rowVersion, &st.Rating, &st.Uncertainty, &st.Runs, &st.SumTargets, &st.Prior)
	if errors.Is(err, pgx.ErrNoRows) {
		return skillrating.State{}, nil, nil
	}
	if err != nil {
		return skillrating.State{}, nil, err
	}
	before := st.Rating
	if rowVersion != versionID {
		st = skillrating.NewVersion(st)
	}
	return st, &before, nil
}

func (s *Service) saveState(ctx context.Context, tx pgx.Tx, agentID, skill, versionID string, st skillrating.State) error {
	_, err := tx.Exec(ctx, `INSERT INTO skill_ratings (agent_id, skill_slug, version_id, rating, uncertainty, runs, sum_targets, prior_rating, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (agent_id, skill_slug) DO UPDATE SET version_id = $3, rating = $4, uncertainty = $5, runs = $6, sum_targets = $7, prior_rating = $8, updated_at = now()`,
		agentID, skill, versionID, st.Rating, st.Uncertainty, st.Runs, st.SumTargets, st.Prior)
	return err
}

// OnNewVersion implements agents.VersionListener: every skill rating moves
// to the new version with confidence reset and the old rating as prior.
//
// A run in progress was played against the old version, so it is aborted first (rating untouched, its
// open task expired). The run row is locked before the rating rows, the same order OnProofFinished takes.
func (s *Service) OnNewVersion(ctx context.Context, tx pgx.Tx, agentID, versionID string) error {
	var running string
	err := tx.QueryRow(ctx, `SELECT id FROM qualification_runs WHERE agent_id = $1 AND status = 'running' FOR UPDATE`, agentID).Scan(&running)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil {
		if err := s.abortTx(ctx, tx, running, "version_changed"); err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `SELECT skill_slug, rating, uncertainty, runs, sum_targets, prior_rating FROM skill_ratings WHERE agent_id = $1 FOR UPDATE`, agentID)
	if err != nil {
		return err
	}
	type row struct {
		skill string
		st    skillrating.State
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.skill, &r.st.Rating, &r.st.Uncertainty, &r.st.Runs, &r.st.SumTargets, &r.st.Prior); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	rows.Close()
	for _, r := range all {
		if err := s.saveState(ctx, tx, agentID, r.skill, versionID, skillrating.NewVersion(r.st)); err != nil {
			return err
		}
	}
	return nil
}

// RatingsFor lists the agent's skill ratings with derived fields. The
// version a rating is shown against is the one it was earned on: that of
// the latest scored run of the skill (skill_ratings.version_id moves to a
// new version as soon as the agent changes, before any run there). Verified
// requires that version to be the agent's current one.
func (s *Service) RatingsFor(ctx context.Context, agentID string) ([]SkillRating, error) {
	out := []SkillRating{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT r.skill_slug, r.rating, r.uncertainty, r.runs, ev.id, ev.number, r.prior_rating,
			coalesce(a.current_version_id = ev.id, false)
			FROM skill_ratings r JOIN agents a ON a.id = r.agent_id
			CROSS JOIN LATERAL (SELECT v.id, v.number FROM qualification_runs q JOIN agent_versions v ON v.id = q.version_id
			  WHERE q.agent_id = r.agent_id AND q.skill_slug = r.skill_slug AND q.status = 'scored'
			  ORDER BY q.finished_at DESC LIMIT 1) ev
			WHERE r.agent_id = $1 ORDER BY r.skill_slug`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r SkillRating
			if err := rows.Scan(&r.SkillSlug, &r.Rating, &r.Uncertainty, &r.Runs, &r.VersionID, &r.VersionNumber, &r.PriorRating, &r.OnCurrentVersion); err != nil {
				return err
			}
			r.Access = skillrating.Access(r.Rating, r.Uncertainty)
			r.Tier = skillrating.Tier(r.Access)
			r.Verified = r.OnCurrentVersion && skillrating.Verified(r.Rating, r.Uncertainty)
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}
