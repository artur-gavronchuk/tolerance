package qualifications

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/proofs"
	"tolerance/internal/skillrating"
)

// OnProofFinished implements proofs.FinishListener: it moves the run to
// the next task, re-queues a task that hit an infra error once, and scores
// the run after the last one. Non-qualification proofs are ignored. It is
// idempotent: only the run's newest proof, once finished, moves the run, so
// the worker's hook and SweepStalled may both fire for the same proof.
func (s *Service) OnProofFinished(ctx context.Context, proofID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, err := s.proofs.GetByIDTx(ctx, tx, proofID)
		if err != nil {
			return err
		}
		if p.Kind != proofs.KindQualification || p.QualificationRunID == nil {
			return nil
		}
		var run Run
		if err := scanRun(tx.QueryRow(ctx, `SELECT `+runCols+` FROM qualification_runs WHERE id = $1 FOR UPDATE`, *p.QualificationRunID), &run); err != nil {
			return err
		}
		if run.Status != StatusRunning || p.FinishedAt == nil {
			return nil
		}
		var newest string
		if err := tx.QueryRow(ctx, `SELECT id FROM proofs WHERE qualification_run_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1`, run.ID).Scan(&newest); err != nil {
			return err
		}
		if newest != p.ID {
			return nil // already moved on
		}
		switch p.Status {
		case proofs.StatusExpired:
			return s.abortTx(ctx, tx, run.ID, "task_expired")
		case proofs.StatusInfraError:
			// retried_infra marks the platform's one re-run of a task: when that
			// re-run also hits an infra error, the task is excluded and the
			// run moves on instead of re-queuing it forever.
			var retried bool
			if err := tx.QueryRow(ctx, `SELECT retried_infra FROM proofs WHERE id = $1`, p.ID).Scan(&retried); err != nil {
				return err
			}
			if !retried {
				again, err := s.proofs.CreateQualificationProof(ctx, tx, run.AgentID, run.ID, *p.SkillTaskSlug, *p.Position)
				if err != nil {
					return err
				}
				_, err = tx.Exec(ctx, `UPDATE proofs SET retried_infra = true WHERE id = $1`, again.ID)
				return err
			}
		}
		if *p.Position < tasksPerRun {
			next := *p.Position + 1
			_, err := s.proofs.CreateQualificationProof(ctx, tx, run.AgentID, run.ID, run.TaskSlugs[next-1], next)
			return err
		}
		return s.scoreTx(ctx, tx, run)
	})
}

// SweepStalled moves on runs whose newest proof finished over a minute ago
// with nothing after it: the worker's hook never ran for it (the API stopped
// between the two transactions) or the proof was closed outside the worker
// (FailOversized). cmd/api calls it every 30 seconds.
func (s *Service) SweepStalled(ctx context.Context) (int, error) {
	var ids []string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT newest.id FROM qualification_runs q
			CROSS JOIN LATERAL (SELECT id, finished_at FROM proofs WHERE qualification_run_id = q.id ORDER BY created_at DESC, id DESC LIMIT 1) newest
			WHERE q.status = 'running' AND newest.finished_at < now() - interval '1 minute'`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	advanced := 0
	var errs []error
	for _, id := range ids {
		if err := s.OnProofFinished(ctx, id); err != nil {
			errs = append(errs, fmt.Errorf("proof %s: %w", id, err))
			continue
		}
		advanced++
	}
	return advanced, errors.Join(errs...)
}

// scoreTx computes the run score from the last terminal proof per position
// and folds it into the skill rating. A task scores the share of its hidden
// tests, by name, that passed; a test outside the hidden set (a visible one,
// or one the agent wrote) never counts, and a name reported both passed and
// failed counts as failed.
func (s *Service) scoreTx(ctx context.Context, tx pgx.Tx, run Run) error {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (p.position) p.position, p.status, p.sandbox_result, t.hidden_tests, t.difficulty, t.hidden_tar, s.language
		FROM proofs p JOIN skill_tasks t ON t.slug = p.skill_task_slug JOIN skills s ON s.slug = t.skill_slug
		WHERE p.qualification_run_id = $1 AND p.finished_at IS NOT NULL
		ORDER BY p.position, p.finished_at DESC`, run.ID)
	if err != nil {
		return err
	}
	var weighted, weights float64
	for rows.Next() {
		var position, hidden, difficulty int
		var status, language string
		var hiddenTar []byte
		var sr *proofs.SandboxResult
		if err := rows.Scan(&position, &status, &sr, &hidden, &difficulty, &hiddenTar, &language); err != nil {
			rows.Close()
			return err
		}
		if status == proofs.StatusInfraError {
			continue // excluded: the platform failed, not the agent
		}
		names, err := proofs.HiddenTestNames(language, hiddenTar)
		if err != nil {
			rows.Close()
			return err
		}
		passed := 0
		if sr != nil {
			ok := map[string]bool{}
			for _, t := range sr.Tests {
				prev, seen := ok[t.Name]
				ok[t.Name] = t.Passed && (!seen || prev)
			}
			for _, n := range names {
				if ok[n] {
					passed++
				}
			}
		}
		if hidden <= 0 {
			rows.Close()
			return fmt.Errorf("qualifications: task at position %d has no hidden tests", position)
		}
		if passed > hidden {
			passed = hidden
		}
		w := float64(difficulty)
		weighted += w * float64(passed) / float64(hidden)
		weights += w
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if weights == 0 {
		return s.abortTx(ctx, tx, run.ID, "no_scored_tasks")
	}
	// Four decimals is what qualification_runs.score stores; the rating is derived from that same value.
	score := math.Round(weighted/weights*1e4) / 1e4

	st, before, err := s.loadState(ctx, tx, run.AgentID, run.SkillSlug, run.VersionID)
	if err != nil {
		return err
	}
	next := skillrating.Apply(st, score)
	if err := s.saveState(ctx, tx, run.AgentID, run.SkillSlug, run.VersionID, next); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE qualification_runs SET status = 'scored', finished_at = now(), score = $2, rating_before = $3, rating_after = $4, uncertainty_after = $5 WHERE id = $1`,
		run.ID, score, before, next.Rating, next.Uncertainty)
	return err
}
