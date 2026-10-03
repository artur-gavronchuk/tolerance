package products

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/httpx"
)

// Results is a task's public standings: one entry per person, ranked by the task kind's rule (see rankRule).
// Before the deadline the entries are hidden and only the phase and counts are returned.
type Results struct {
	Task    Task    `json:"task"`
	Entries []Entry `json:"entries"`
}

const votesOf = `(SELECT count(*) FROM product_votes v WHERE v.entry_id = e.id)`

// rankRule is how a task's entries are counted and ordered.
//   - cli: a person's best upload counts (most scenarios passed, earliest on ties). Ranked by scenarios passed,
//     then votes: a tool that fails checks does not win on popularity, and the votes pick among the correct ones.
//   - site: a person's latest upload counts. Ranked by votes, then automated checks (when the task has them).
//
// Both end on the earlier upload.
func rankRule(kind string) (pick, order string) {
	if kind == KindSite {
		return "created_at DESC", votesOf + " DESC, e.passed DESC, e.created_at"
	}
	return "passed DESC, created_at", "e.passed DESC, " + votesOf + " DESC, e.created_at"
}

func (s *Service) Results(ctx context.Context, slug, userID string) (Results, error) {
	r := Results{Entries: []Entry{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		r.Task, err = scanTask(tx.QueryRow(ctx, `SELECT `+taskCols+` FROM product_tasks t WHERE t.slug = $1 AND t.active AND t.opens_at <= now()`, slug))
		if err != nil || r.Task.Phase == PhaseOpen {
			return err
		}
		pick, order := rankRule(r.Task.Kind)
		rows, err := tx.Query(ctx, `
			SELECT `+entryCols+` FROM (
				SELECT DISTINCT ON (user_id) * FROM product_entries WHERE task_slug = $2 AND status = 'done'
				ORDER BY user_id, `+pick+`) e
			JOIN users u ON u.id = e.user_id
			ORDER BY `+order, userID, slug)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			e, err := scanEntry(rows)
			if err != nil {
				return err
			}
			e.LogTail = "" // the participant's own output stays with them
			r.Entries = append(r.Entries, e)
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Results{}, httpx.NotFound()
	}
	return r, err
}

// Vote records the caller's vote for a published entry. A person has one vote per task: voting for another
// entry moves it. Never for your own entry, and only while the voting window is open.
func (s *Service) Vote(ctx context.Context, userID, entryID string) (Entry, error) {
	var out Entry
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		slug, kind, owner, deadline, err := votable(ctx, tx, entryID)
		if err != nil {
			return err
		}
		if err := checkVoting(deadline); err != nil {
			return err
		}
		if owner == userID {
			return httpx.New(http.StatusForbidden, "own_entry", "You cannot vote for your own entry")
		}
		// Only the upload that stands for its author in the results can be voted for.
		pick, _ := rankRule(kind)
		var counted string
		if err := tx.QueryRow(ctx, `SELECT id FROM product_entries e WHERE task_slug = $1 AND user_id = $2 AND status = 'done'
			ORDER BY `+pick+` LIMIT 1`, slug, owner).Scan(&counted); err != nil {
			return err
		}
		if counted != entryID {
			return httpx.New(http.StatusConflict, "not_counted", "That upload is not the one shown in the results")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO product_votes (entry_id, user_id, task_slug) VALUES ($1, $2, $3)
			ON CONFLICT (task_slug, user_id) DO UPDATE SET entry_id = EXCLUDED.entry_id, created_at = now()`, entryID, userID, slug); err != nil {
			return err
		}
		out, err = scanEntry(tx.QueryRow(ctx, `SELECT `+entryCols+` FROM product_entries e JOIN users u ON u.id = e.user_id WHERE e.id = $2`, userID, entryID))
		return err
	})
	return out, err
}

// Unvote takes the caller's vote back from an entry while the voting window is open.
func (s *Service) Unvote(ctx context.Context, userID, entryID string) (Entry, error) {
	var out Entry
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, _, _, deadline, err := votable(ctx, tx, entryID)
		if err != nil {
			return err
		}
		if err := checkVoting(deadline); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM product_votes WHERE entry_id = $1 AND user_id = $2`, entryID, userID); err != nil {
			return err
		}
		out, err = scanEntry(tx.QueryRow(ctx, `SELECT `+entryCols+` FROM product_entries e JOIN users u ON u.id = e.user_id WHERE e.id = $2`, userID, entryID))
		return err
	})
	return out, err
}

func votable(ctx context.Context, tx pgx.Tx, entryID string) (slug, kind, owner string, deadline time.Time, err error) {
	var status string
	err = tx.QueryRow(ctx, `SELECT e.task_slug, t.kind, e.user_id, t.deadline, e.status FROM product_entries e JOIN product_tasks t ON t.slug = e.task_slug
		WHERE e.id = $1`, entryID).Scan(&slug, &kind, &owner, &deadline, &status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != StatusDone) {
		return "", "", "", time.Time{}, httpx.NotFound()
	}
	return
}

func checkVoting(deadline time.Time) error {
	switch phaseOf(deadline) {
	case PhaseOpen:
		return httpx.New(http.StatusConflict, "voting_not_open", "Voting opens after the deadline")
	case PhaseFinal:
		return httpx.New(http.StatusConflict, "voting_closed", "Voting is over; the results are final")
	}
	return nil
}

// Close ends a task's uploads now and starts voting; with final it also ends the voting window. Reopen starts
// the clock again. Both are for admins and local runs, so the post-deadline half can be tried at all.
func (s *Service) Close(ctx context.Context, actorID, slug string, final bool) error {
	return s.setDeadline(ctx, actorID, slug, "product_task.closed", func(old time.Time) time.Time {
		now := time.Now().UTC()
		if final {
			return minTime(old, now.Add(-VotingWindow-time.Minute))
		}
		return minTime(old, now)
	})
}

func (s *Service) Reopen(ctx context.Context, actorID, slug string, days int) error {
	if days < 1 || days > 60 {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_days", "days must be between 1 and 60", "days", "invalid")
	}
	return s.setDeadline(ctx, actorID, slug, "product_task.reopened", func(time.Time) time.Time {
		return time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
	})
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (s *Service) setDeadline(ctx context.Context, actorID, slug, action string, next func(old time.Time) time.Time) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var old time.Time
		if err := tx.QueryRow(ctx, `SELECT deadline FROM product_tasks WHERE slug = $1 AND active FOR UPDATE`, slug).Scan(&old); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE product_tasks SET deadline = $2 WHERE slug = $1`, slug, next(old)); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, Action: action, AggregateKind: "product_task", AggregateID: slug,
			RequestID: httpx.RequestID(ctx)})
	})
}
