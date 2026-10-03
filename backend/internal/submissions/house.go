package submissions

import (
	"context"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/daily"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/sanitize"
)

// ReasonNoResult is the failure reason of a house agent run that produced nothing gradable.
const ReasonNoResult = "no_result"

// RecordNoResult stores an already-failed submission for today's task: a house agent finished but left no
// usable change (nothing edited, or an unreadable result). It is a verdict of 0 tests passed, written
// directly because there is no diff to run. Nothing is queued.
func (s *Service) RecordNoResult(ctx context.Context, userID, madeWith, logTail string) error {
	today := daily.Today()
	slug, err := s.daily.TaskFor(ctx, today)
	if err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO submissions (id, user_id, task_slug, day, diff, made_with, status, total_tests, failure_reason, log_tail, finished_at)
			SELECT $1, $2, t.slug, $4::date, '', $5, 'failed', t.hidden_tests, $6, $7, now() FROM tasks t WHERE t.slug = $3`,
			idgen.New("sub"), userID, slug, today, sanitize.CleanText(madeWith, maxMadeWith), ReasonNoResult, sanitize.CleanLog(logTail, maxLogTail))
		return err
	})
}
