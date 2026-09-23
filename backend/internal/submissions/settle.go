package submissions

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/competitions"
	"tolerance/internal/platform/httpx"
)

// Ref is what the checks module needs to know about a submission to run and
// score its checks. It is a read of a locked row: the caller holds the
// submission for the rest of its transaction.
type Ref struct {
	ID                string
	CompetitionID     string
	AgentID           string
	AttemptID         *string
	AttemptKind       string
	ScoreStatus       string
	CheckRunID        *string
	PreviewURL        string
	RepoURL           string
	CommitSHA         string
	CompetitionPoints int
	Criteria          []competitions.Criterion
}

// Lock loads a submission with the fields checks need and locks its row.
func (s *Service) Lock(ctx context.Context, tx pgx.Tx, id string) (Ref, error) {
	var r Ref
	var previewURL, repoURL, sha *string
	var criteria []byte
	err := tx.QueryRow(ctx, `SELECT s.id, s.competition_id, s.agent_id, s.attempt_id, s.attempt_kind, s.score_status, s.check_run_id,
			s.preview_url, s.repo_url, s.commit_sha, c.points, c.criteria
		FROM submissions s JOIN competitions c ON c.id = s.competition_id
		WHERE s.id = $1 FOR UPDATE OF s`, id).
		Scan(&r.ID, &r.CompetitionID, &r.AgentID, &r.AttemptID, &r.AttemptKind, &r.ScoreStatus, &r.CheckRunID,
			&previewURL, &repoURL, &sha, &r.CompetitionPoints, &criteria)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ref{}, httpx.NotFound()
	}
	if err != nil {
		return Ref{}, err
	}
	if previewURL != nil {
		r.PreviewURL = *previewURL
	}
	if repoURL != nil {
		r.RepoURL = *repoURL
	}
	if sha != nil {
		r.CommitSHA = *sha
	}
	if err := json.Unmarshal(criteria, &r.Criteria); err != nil {
		return Ref{}, err
	}
	return r, nil
}

// SetChecking marks a submission as being checked. A scored submission is
// left alone: it only goes back to checking through an explicit recheck.
func (s *Service) SetChecking(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := tx.Exec(ctx, `UPDATE submissions SET score_status = 'judging', version = version + 1
		WHERE id = $1 AND score_status <> 'scored'`, id)
	return err
}

// Recheck sends any submission, scored or not, back to checking with its
// new check run. Until the new run finishes its old score stays visible but
// the submission is no longer counted as scored.
func (s *Service) Recheck(ctx context.Context, tx pgx.Tx, id, checkRunID string) error {
	_, err := tx.Exec(ctx, `UPDATE submissions SET score_status = 'judging', check_run_id = $2, unscored_reason = NULL, version = version + 1
		WHERE id = $1`, id, checkRunID)
	return err
}

// SetCommitLink records how the deployment's declared commit compares with
// the submitted one. There is deliberately no "verified" value.
func (s *Service) SetCommitLink(ctx context.Context, tx pgx.Tx, id, link string) error {
	switch link {
	case "declared_match", "mismatch", "unverified":
	default:
		return errors.New("submissions: invalid commit link " + link)
	}
	_, err := tx.Exec(ctx, `UPDATE submissions SET commit_link = $2 WHERE id = $1`, id, link)
	return err
}

// ScoreUpdate is a complete official result.
type ScoreUpdate struct {
	Scores        []ScoreEntry
	Total         int
	PointsAwarded int
	JudgmentID    *string
}

// ApplyScore makes the update the submission's official result. Calling it
// again replaces the previous result field by field, so it can never add
// points on top of an earlier one.
func (s *Service) ApplyScore(ctx context.Context, tx pgx.Tx, id string, u ScoreUpdate) error {
	scores, err := json.Marshal(u.Scores)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE submissions SET score_status = 'scored', scores = $2, total = $3, points_awarded = $4,
			judgment_id = $5, judged_at = now(), unscored_reason = NULL, version = version + 1
		WHERE id = $1`, id, scores, u.Total, u.PointsAwarded, u.JudgmentID)
	return err
}

func (s *Service) markUnscored(ctx context.Context, tx pgx.Tx, id, status, reason string) error {
	_, err := tx.Exec(ctx, `UPDATE submissions SET score_status = $2, unscored_reason = $3, scores = NULL, total = NULL,
			points_awarded = NULL, judgment_id = NULL, judged_at = NULL, version = version + 1
		WHERE id = $1`, id, status, reason)
	return err
}

// MarkUnverifiable records that the work could not be opened by the checker.
// It is not scored from its description: no total, no points.
func (s *Service) MarkUnverifiable(ctx context.Context, tx pgx.Tx, id, reason string) error {
	return s.markUnscored(ctx, tx, id, StatusUnverifiable, reason)
}

// MarkFailed records a platform failure. It is not a result and not a loss.
func (s *Service) MarkFailed(ctx context.Context, tx pgx.Tx, id, reason string) error {
	return s.markUnscored(ctx, tx, id, StatusFailed, reason)
}
