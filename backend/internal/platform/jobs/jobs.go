// Package jobs is a small durable work queue on the jobs table. Workers
// claim with FOR UPDATE SKIP LOCKED under a lease; a job whose worker
// disappears is returned to the queue by Reclaim, and one that keeps
// failing ends up in state failed where an admin can retry it.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

const maxErrorLen = 2000

type Job struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	State       string          `json:"state"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	Payload     json.RawMessage `json:"payload"`
	LastError   string          `json:"last_error"`
	RunAfter    time.Time       `json:"run_after"`
	CreatedAt   time.Time       `json:"created_at"`
}

const cols = `id, kind, state, attempts, max_attempts, payload, coalesce(last_error, ''), run_after, created_at`

func scan(row pgx.Row, j *Job) error {
	err := row.Scan(&j.ID, &j.Kind, &j.State, &j.Attempts, &j.MaxAttempts, &j.Payload, &j.LastError, &j.RunAfter, &j.CreatedAt)
	j.RunAfter, j.CreatedAt = j.RunAfter.UTC(), j.CreatedAt.UTC()
	return err
}

// Enqueue inserts a job in the caller's transaction, so it is created if
// and only if the caller's change commits. A non-empty dedupeKey makes the
// call idempotent: an existing job with that key is returned instead.
func Enqueue(ctx context.Context, tx pgx.Tx, kind string, payload any, dedupeKey string) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("jobs: marshal payload: %w", err)
	}
	id := idgen.New("job")
	err = tx.QueryRow(ctx, `
		INSERT INTO jobs (id, kind, dedupe_key, payload) VALUES ($1, $2, NULLIF($3, ''), $4)
		ON CONFLICT (dedupe_key) DO NOTHING RETURNING id`, id, kind, dedupeKey, body).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM jobs WHERE dedupe_key = $1`, dedupeKey).Scan(&id)
	}
	if err != nil {
		return "", fmt.Errorf("jobs: enqueue %s: %w", kind, err)
	}
	return id, nil
}

type Queue struct{ pool *db.Pool }

func New(pool *db.Pool) *Queue { return &Queue{pool: pool} }

// Claim leases the oldest runnable job of one of the given kinds, or
// returns nil when there is none. Each claim counts as an attempt.
func (q *Queue) Claim(ctx context.Context, owner string, kinds []string, lease time.Duration) (*Job, error) {
	var j Job
	err := q.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scan(tx.QueryRow(ctx, `
			UPDATE jobs SET state = 'leased', lease_owner = $1,
			       lease_until = now() + make_interval(secs => $3), attempts = attempts + 1
			WHERE id = (SELECT id FROM jobs
			            WHERE state = 'queued' AND run_after <= now() AND kind = ANY($2)
			            ORDER BY run_after FOR UPDATE SKIP LOCKED LIMIT 1)
			RETURNING `+cols, owner, kinds, lease.Seconds()), &j)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("jobs: claim: %w", err)
	}
	return &j, nil
}

// Complete marks a leased job done. A job whose lease was lost in the
// meantime is left alone.
func (q *Queue) Complete(ctx context.Context, id string) error {
	return q.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE jobs SET state = 'done', lease_owner = NULL, lease_until = NULL
			WHERE id = $1 AND state = 'leased'`, id)
		return err
	})
}

// Fail records the error and either schedules a retry (30 s, 2 min, then
// 10 min) or, once max_attempts is used up, parks the job as failed.
func (q *Queue) Fail(ctx context.Context, id string, cause error) (final bool, err error) {
	msg := cause.Error()
	if len(msg) > maxErrorLen {
		msg = msg[:maxErrorLen]
	}
	err = q.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var state string
		err := tx.QueryRow(ctx, `
			UPDATE jobs SET
			  state = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
			  run_after = CASE WHEN attempts >= max_attempts THEN run_after ELSE now() +
			      CASE attempts WHEN 1 THEN interval '30 seconds' WHEN 2 THEN interval '2 minutes' ELSE interval '10 minutes' END END,
			  last_error = $2, lease_owner = NULL, lease_until = NULL
			WHERE id = $1 AND state = 'leased' RETURNING state`, id, msg).Scan(&state)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // lease already lost; whoever holds the job now decides its fate
		}
		final = state == "failed"
		return err
	})
	return final, err
}

// Reclaim returns jobs whose lease expired to the queue, or parks them as
// failed when they have already used every attempt. It reports how many
// jobs it touched.
func (q *Queue) Reclaim(ctx context.Context) (int, error) {
	var n int64
	err := q.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE jobs SET
			  state = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
			  last_error = coalesce(last_error, 'worker lease expired'),
			  lease_owner = NULL, lease_until = NULL
			WHERE state = 'leased' AND lease_until < now()`)
		n = tag.RowsAffected()
		return err
	})
	return int(n), err
}

// List returns the most recent jobs in the given state (all states when empty).
func (q *Queue) List(ctx context.Context, state string) ([]Job, error) {
	out := []Job{}
	err := q.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+cols+` FROM jobs WHERE ($1 = '' OR state = $1)
			ORDER BY created_at DESC, id LIMIT 100`, state)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var j Job
			if err := scan(rows, &j); err != nil {
				return err
			}
			out = append(out, j)
		}
		return rows.Err()
	})
	return out, err
}

// Retry puts a failed job back in the queue with a fresh attempt counter.
func (q *Queue) Retry(ctx context.Context, id string) error {
	return q.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM jobs WHERE id = $1 FOR UPDATE`, id).Scan(&state); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		if state != "failed" {
			return httpx.StateConflict("Only failed jobs can be retried")
		}
		_, err := tx.Exec(ctx, `UPDATE jobs SET state = 'queued', attempts = 0, run_after = now(),
			lease_owner = NULL, lease_until = NULL WHERE id = $1`, id)
		return err
	})
}
