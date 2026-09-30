package skills

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/httpx"
)

// MinPool is the default number of issuable tasks a skill needs before it will
// start new runs. Below it the skill freezes: handing everyone the same few
// already-leaked tasks is worse than saying out loud that the pool is being
// refilled. The effective floor is configurable (ARENA_SKILL_MIN_POOL) because
// the public practice catalog in this repository is deliberately smaller than
// the private rating catalog the production server mounts.
const MinPool = 5

// MaxExposures is how many distinct agents may see a task before it leaves the
// pool. It is a dial, not a truth — the repository leaves the platform on every
// run, so the only question is how wide the leak gets before results stop
// counting toward a rating.
const MaxExposures = 40

// ErrFrozen is the 409 a caller gets for a skill with too few issuable tasks.
func ErrFrozen() error {
	return httpx.New(http.StatusConflict, "skill_frozen",
		"This skill is being refilled with fresh tasks; try again later")
}

// Frozen reports whether a pool this small is too small to run on. A floor of
// zero switches the policy off, but an empty pool is always frozen: a run cannot
// be built out of no tasks, and letting one through would fail deep inside the
// insert and reach the owner as a 500 instead of an explanation.
func Frozen(issuable, min int) bool { return issuable == 0 || issuable < min }

// Pool returns the slugs a qualification run may be built from: present in the
// mounted catalog, not retired, not reserved for a challenge. Ordered by slug so
// a pick with the same rand seed is reproducible.
func Pool(ctx context.Context, tx pgx.Tx, skill string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT slug FROM skill_tasks
		WHERE skill_slug = $1 AND active AND retired_at IS NULL AND NOT challenge_only ORDER BY slug`, skill)
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

// RecordExposure notes that agentID has seen taskSlug and reports whether this
// was the first time. skill_tasks.exposures is the denormalized count of
// distinct agents, so it only moves on a first sighting — a task re-issued to
// the same agent after an infra error does not widen the leak. The task retires
// itself inside the same UPDATE once the count reaches MaxExposures, so no
// scheduler has to notice, and a run already in flight on it still finishes:
// retirement only keeps the task out of future picks.
func RecordExposure(ctx context.Context, tx pgx.Tx, taskSlug, agentID string) (bool, error) {
	var first bool
	err := tx.QueryRow(ctx, `INSERT INTO skill_task_exposures (task_slug, agent_id)
		VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING true`, taskSlug, agentID).Scan(&first)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE skill_tasks SET
		exposures = exposures + 1,
		first_used_at = coalesce(first_used_at, now()),
		retired_at = CASE WHEN retired_at IS NULL AND exposures + 1 >= $2 THEN now() ELSE retired_at END,
		retired_reason = CASE WHEN retired_at IS NULL AND exposures + 1 >= $2
			THEN 'exposure limit reached' ELSE retired_reason END
		WHERE slug = $1`, taskSlug, MaxExposures)
	return true, err
}

// TaskStat is what an administrator needs in order to judge a task: how far it
// has leaked, and how hard it turned out to be next to the difficulty someone
// typed by hand. A wide gap between the two is the signal to re-weight — by
// editing difficulty, which affects only future runs. Nothing here rewrites a
// rating that has already been earned.
type TaskStat struct {
	Slug          string     `json:"slug"`
	Title         string     `json:"title"`
	Difficulty    int        `json:"difficulty"`
	Exposures     int        `json:"exposures"`
	Runs          int        `json:"runs"`
	AvgScore      *float64   `json:"avg_score"`
	Active        bool       `json:"active"`
	ChallengeOnly bool       `json:"challenge_only"`
	FirstUsedAt   *time.Time `json:"first_used_at"`
	RetiredAt     *time.Time `json:"retired_at"`
	RetiredReason string     `json:"retired_reason"`
}

// Retire takes a task out of the pool by hand, for a leak the exposure counter
// cannot see. Results already scored on it keep counting: retiring a task never
// changes a rating that was earned on it, and a run already in flight finishes.
// Retiring an already-retired task keeps the reason it was retired the first time.
func Retire(ctx context.Context, tx pgx.Tx, slug, reason string) error {
	tag, err := tx.Exec(ctx, `UPDATE skill_tasks SET retired_at = coalesce(retired_at, now()),
		retired_reason = CASE WHEN retired_at IS NULL THEN $2 ELSE retired_reason END
		WHERE slug = $1`, slug, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound()
	}
	return nil
}

// TaskStats lists one skill's whole catalog, retired tasks included.
func TaskStats(ctx context.Context, tx pgx.Tx, skill string) ([]TaskStat, error) {
	rows, err := tx.Query(ctx, `SELECT slug, title, difficulty, exposures, runs,
			CASE WHEN runs > 0 THEN (sum_score / runs)::float8 ELSE NULL END,
			active, challenge_only, first_used_at, retired_at, retired_reason
		FROM skill_tasks WHERE skill_slug = $1 ORDER BY slug`, skill)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskStat{}
	for rows.Next() {
		var s TaskStat
		if err := rows.Scan(&s.Slug, &s.Title, &s.Difficulty, &s.Exposures, &s.Runs, &s.AvgScore,
			&s.Active, &s.ChallengeOnly, &s.FirstUsedAt, &s.RetiredAt, &s.RetiredReason); err != nil {
			return nil, err
		}
		s.FirstUsedAt, s.RetiredAt = utcp(s.FirstUsedAt), utcp(s.RetiredAt)
		out = append(out, s)
	}
	return out, rows.Err()
}

// utcp normalizes a nullable timestamp; pgx decodes timestamptz into time.Local.
func utcp(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
