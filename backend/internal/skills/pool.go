package skills

import (
	"context"
	"errors"
	"net/http"

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
