package challenges

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/proofs"
)

// OnProofFinished records an entry's result. It is wired into the proofs worker
// next to the qualification hook and ignores every proof that is not a
// challenge entry, so both can be installed at once.
//
// The score is the fraction of the task's hidden tests that ran and passed, by
// name — the same rule a qualification run is scored by, so a diff cannot earn a
// place with tests it invented. An infra error records nothing: the platform
// failed, and Close will treat the entry as unfinished only if it never recovers.
func (s *Service) OnProofFinished(ctx context.Context, proofID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var kind, status, diff string
		var challengeID *string
		var taskSlug *string
		var sr *proofs.SandboxResult
		err := tx.QueryRow(ctx, `SELECT kind, status, challenge_id, skill_task_slug, coalesce(diff, ''), sandbox_result
			FROM proofs WHERE id = $1 FOR UPDATE`, proofID).Scan(&kind, &status, &challengeID, &taskSlug, &diff, &sr)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if kind != proofs.KindChallenge || challengeID == nil || taskSlug == nil {
			return nil
		}
		if status == proofs.StatusInfraError {
			return nil
		}

		score := 0.0
		if status == proofs.StatusPassed || status == proofs.StatusFailed {
			score, err = s.hiddenFraction(ctx, tx, *taskSlug, sr)
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE challenge_entries SET score = $3, diff_lines = $4, submitted_at = now()
			WHERE challenge_id = $1 AND proof_id = $2`, *challengeID, proofID, score, diffLines(diff))
		return err
	})
}

// hiddenFraction is how much of the task's hidden suite actually ran and passed.
func (s *Service) hiddenFraction(ctx context.Context, tx pgx.Tx, taskSlug string, sr *proofs.SandboxResult) (float64, error) {
	var language string
	var hiddenTar []byte
	if err := tx.QueryRow(ctx, `SELECT s.language, t.hidden_tar FROM skill_tasks t JOIN skills s ON s.slug = t.skill_slug
		WHERE t.slug = $1`, taskSlug).Scan(&language, &hiddenTar); err != nil {
		return 0, err
	}
	names, err := proofs.HiddenTestNames(language, hiddenTar)
	if err != nil {
		return 0, err
	}
	if len(names) == 0 || sr == nil {
		return 0, nil
	}
	ok := map[string]bool{}
	for _, t := range sr.Tests {
		prev, seen := ok[t.Name]
		ok[t.Name] = t.Passed && (!seen || prev)
	}
	passed := 0
	for _, n := range names {
		if ok[n] {
			passed++
		}
	}
	return float64(passed) / float64(len(names)), nil
}

// diffLines counts the lines a diff adds or removes, ignoring its file headers.
// It is a tie-breaker, not a quality score: smaller wins, and an entrant can
// count it themselves.
func diffLines(diff string) int {
	n := 0
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			n++
		}
	}
	return n
}

// Open starts a challenge early; Tick does it by the clock.
func (s *Service) Open(ctx context.Context, actorID, slug string) error {
	return s.transition(ctx, actorID, slug, StatusDraft, StatusOpen, "challenge_not_draft", "admin.challenge_opened")
}

// Publish opens up a closed challenge: its task, its hidden test names and the
// diffs of entrants who agreed. A challenge burns its task, and publication is
// what makes that worth something to everyone who reads it afterwards.
func (s *Service) Publish(ctx context.Context, actorID, slug string) error {
	return s.transition(ctx, actorID, slug, StatusClosed, StatusPublished, "challenge_not_closed", "admin.challenge_published")
}

func (s *Service) transition(ctx context.Context, actorID, slug, from, to, code, action string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var c Challenge
		if err := scanChallenge(tx.QueryRow(ctx, `SELECT `+challengeCols+` FROM challenges WHERE slug = $1 FOR UPDATE`, slug), &c); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		if c.Status != from {
			return httpx.New(http.StatusConflict, code, "This challenge is "+c.Status)
		}
		if _, err := tx.Exec(ctx, `UPDATE challenges SET status = $2 WHERE id = $1`, c.ID, to); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, ActorKind: identity.KindUser, Action: action,
			AggregateKind: "challenge", AggregateID: c.ID, Payload: map[string]any{"slug": slug}, RequestID: httpx.RequestID(ctx)})
	})
}

// Close ends a challenge and hands out places. An entry whose proof never
// reached a verdict scores zero and ranks behind everyone who did: the deadline
// passed, so that attempt did not happen. Its proof is expired in the same
// transaction, which also frees the agent's one-open-proof slot.
func (s *Service) Close(ctx context.Context, actorID, slug string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var c Challenge
		if err := scanChallenge(tx.QueryRow(ctx, `SELECT `+challengeCols+` FROM challenges WHERE slug = $1 FOR UPDATE`, slug), &c); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		if c.Status != StatusOpen {
			return httpx.New(http.StatusConflict, "challenge_not_open", "This challenge is "+c.Status)
		}
		if _, err := tx.Exec(ctx, `UPDATE proofs SET status = 'expired', finished_at = now(), failure_reason = 'challenge closed'
			WHERE challenge_id = $1 AND finished_at IS NULL`, c.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE challenge_entries SET score = 0, submitted_at = coalesce(submitted_at, now())
			WHERE challenge_id = $1 AND score IS NULL`, c.ID); err != nil {
			return err
		}

		rows, err := tx.Query(ctx, `SELECT a.name, e.score::float8, coalesce(e.diff_lines, 0), e.submitted_at
			FROM challenge_entries e JOIN agents a ON a.id = e.agent_id WHERE e.challenge_id = $1`, c.ID)
		if err != nil {
			return err
		}
		var results []Result
		for rows.Next() {
			var r Result
			if err := rows.Scan(&r.AgentName, &r.Score, &r.DiffLines, &r.SubmittedAt); err != nil {
				rows.Close()
				return err
			}
			results = append(results, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, r := range Rank(results) {
			if _, err := tx.Exec(ctx, `UPDATE challenge_entries e SET rank = $3 FROM agents a
				WHERE a.id = e.agent_id AND e.challenge_id = $1 AND a.name = $2`, c.ID, r.AgentName, r.Rank); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE challenges SET status = 'closed' WHERE id = $1`, c.ID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, ActorKind: identity.KindUser, Action: "admin.challenge_closed",
			AggregateKind: "challenge", AggregateID: c.ID, Payload: map[string]any{"slug": slug, "entrants": len(results)},
			RequestID: httpx.RequestID(ctx)})
	})
}

// Tick moves challenges by the clock: drafts whose start has passed open, and
// open challenges whose deadline has passed close. One failing challenge does not
// stop the others — the next tick tries again.
func (s *Service) Tick(ctx context.Context) error {
	var toOpen, toClose []string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		toOpen, err = slugsWhere(ctx, tx, `status = 'draft' AND opens_at <= now()`)
		if err != nil {
			return err
		}
		toClose, err = slugsWhere(ctx, tx, `status = 'open' AND closes_at <= now()`)
		return err
	})
	if err != nil {
		return err
	}
	var firstErr error
	for _, slug := range toOpen {
		if err := s.Open(ctx, identity.System.ID, slug); err != nil {
			s.log().Error("challenge open tick", "slug", slug, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	for _, slug := range toClose {
		if err := s.Close(ctx, identity.System.ID, slug); err != nil {
			s.log().Error("challenge close tick", "slug", slug, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func slugsWhere(ctx context.Context, tx pgx.Tx, where string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT slug FROM challenges WHERE `+where+` ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}

// SetLogger wires the logger the scheduler tick reports through; without it the
// tick logs to the default logger.
func (s *Service) SetLogger(l *slog.Logger) { s.logger = l }

func (s *Service) log() *slog.Logger {
	if s.logger == nil {
		return slog.Default()
	}
	return s.logger
}

// Patch edits the two fields that stay editable after creation: the prize text
// and the note saying it was paid. Prizes are settled outside the platform —
// there is no money here — so the note is the whole record.
func (s *Service) Patch(ctx context.Context, actorID, slug string, prizes, payoutNote *string) (Challenge, error) {
	var c Challenge
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanChallenge(tx.QueryRow(ctx, `UPDATE challenges SET prizes = coalesce($2, prizes),
			payout_note = coalesce($3, payout_note) WHERE slug = $1 RETURNING `+challengeCols, slug, prizes, payoutNote), &c); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, ActorKind: identity.KindUser, Action: "admin.challenge_patched",
			AggregateKind: "challenge", AggregateID: c.ID, Payload: map[string]any{"slug": slug}, RequestID: httpx.RequestID(ctx)})
	})
	return c, err
}
