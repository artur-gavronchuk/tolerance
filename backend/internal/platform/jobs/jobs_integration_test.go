package jobs_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/jobs"
)

func enqueue(t *testing.T, d *dbtest.DB, kind, dedupe string) string {
	t.Helper()
	var id string
	err := d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, err = jobs.Enqueue(ctx, tx, kind, map[string]string{"n": "1"}, dedupe)
		return err
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return id
}

// pastRunAfter makes a job that was scheduled for later claimable again.
func pastRunAfter(t *testing.T, d *dbtest.DB, id string) {
	t.Helper()
	err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE jobs SET run_after = now() WHERE id = $1`, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestJobs_TwoWorkersNeverShareAJob(t *testing.T) {
	d := dbtest.New(t)
	q := jobs.New(d.AppPool)
	want := map[string]bool{}
	for i := 0; i < 20; i++ {
		want[enqueue(t, d, "run_proof", "")] = true
	}

	var mu sync.Mutex
	got := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			for {
				j, err := q.Claim(context.Background(), owner, []string{"run_proof"}, time.Minute)
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				if j == nil {
					return
				}
				mu.Lock()
				got[j.ID]++
				mu.Unlock()
			}
		}(string(rune('a' + w)))
	}
	wg.Wait()

	if len(got) != 20 {
		t.Fatalf("expected all 20 jobs claimed, got %d", len(got))
	}
	for id, n := range got {
		if n != 1 || !want[id] {
			t.Fatalf("job %s claimed %d times (known=%v)", id, n, want[id])
		}
	}
}

func TestJobs_DedupeKey(t *testing.T) {
	d := dbtest.New(t)
	a := enqueue(t, d, "run_proof", "run:run_1")
	b := enqueue(t, d, "run_proof", "run:run_1")
	if a != b {
		t.Fatalf("same dedupe key must return the existing job: %s vs %s", a, b)
	}
	c := enqueue(t, d, "run_proof", "")
	e := enqueue(t, d, "run_proof", "")
	if c == e {
		t.Fatal("jobs without a dedupe key must all be distinct")
	}
	var n int
	_ = d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&n)
	})
	if n != 3 {
		t.Fatalf("expected 3 rows, got %d", n)
	}
}

func TestJobs_FailSchedule(t *testing.T) {
	d := dbtest.New(t)
	q := jobs.New(d.AppPool)
	ctx := context.Background()
	id := enqueue(t, d, "run_proof", "")

	delays := []time.Duration{30 * time.Second, 2 * time.Minute}
	for i, want := range delays {
		j, err := q.Claim(ctx, "w", []string{"run_proof"}, time.Minute)
		if err != nil || j == nil {
			t.Fatalf("attempt %d: claim: %v %v", i+1, j, err)
		}
		if j.Attempts != i+1 {
			t.Fatalf("attempt counter: got %d want %d", j.Attempts, i+1)
		}
		final, err := q.Fail(ctx, id, errors.New("boom"))
		if err != nil || final {
			t.Fatalf("attempt %d must not be final: %v %v", i+1, final, err)
		}
		if again, _ := q.Claim(ctx, "w", []string{"run_proof"}, time.Minute); again != nil {
			t.Fatalf("attempt %d: job must wait for its backoff", i+1)
		}
		var wait time.Duration
		_ = d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var secs float64
			err := tx.QueryRow(ctx, `SELECT extract(epoch FROM run_after - now()) FROM jobs WHERE id = $1`, id).Scan(&secs)
			wait = time.Duration(secs * float64(time.Second))
			return err
		})
		if wait < want-5*time.Second || wait > want+5*time.Second {
			t.Fatalf("attempt %d: backoff %v, want about %v", i+1, wait, want)
		}
		pastRunAfter(t, d, id)
	}

	if j, _ := q.Claim(ctx, "w", []string{"run_proof"}, time.Minute); j == nil {
		t.Fatal("third attempt must be claimable")
	}
	final, err := q.Fail(ctx, id, errors.New("boom again"))
	if err != nil || !final {
		t.Fatalf("third failure must be final: %v %v", final, err)
	}
	failed, err := q.List(ctx, "failed")
	if err != nil || len(failed) != 1 || failed[0].LastError != "boom again" {
		t.Fatalf("failed list: %+v %v", failed, err)
	}
}

func TestJobs_ReclaimExpiredLease(t *testing.T) {
	d := dbtest.New(t)
	q := jobs.New(d.AppPool)
	ctx := context.Background()
	enqueue(t, d, "run_proof", "")

	if j, err := q.Claim(ctx, "crashed-worker", []string{"run_proof"}, time.Millisecond); err != nil || j == nil {
		t.Fatalf("claim: %v %v", j, err)
	}
	time.Sleep(20 * time.Millisecond)
	if n, err := q.Reclaim(ctx); err != nil || n != 1 {
		t.Fatalf("reclaim: %d %v", n, err)
	}
	j, err := q.Claim(ctx, "healthy-worker", []string{"run_proof"}, time.Minute)
	if err != nil || j == nil {
		t.Fatalf("reclaimed job must be claimable again: %v %v", j, err)
	}
	if j.Attempts != 2 {
		t.Fatalf("a reclaimed job keeps counting attempts, got %d", j.Attempts)
	}
	if n, _ := q.Reclaim(ctx); n != 0 {
		t.Fatal("a live lease must not be reclaimed")
	}
}

func TestJobs_ReclaimGivesUpAfterMaxAttempts(t *testing.T) {
	d := dbtest.New(t)
	q := jobs.New(d.AppPool)
	ctx := context.Background()
	id := enqueue(t, d, "run_proof", "")
	for i := 0; i < 3; i++ {
		if j, err := q.Claim(ctx, "w", []string{"run_proof"}, time.Millisecond); err != nil || j == nil {
			t.Fatalf("claim %d: %v %v", i+1, j, err)
		}
		time.Sleep(20 * time.Millisecond)
		if n, err := q.Reclaim(ctx); err != nil || n != 1 {
			t.Fatalf("reclaim %d: %d %v", i+1, n, err)
		}
	}
	if j, _ := q.Claim(ctx, "w", []string{"run_proof"}, time.Minute); j != nil {
		t.Fatal("a job that always loses its lease must end up failed, not loop forever")
	}
	failed, _ := q.List(ctx, "failed")
	if len(failed) != 1 || failed[0].ID != id {
		t.Fatalf("expected the job in the failed list: %+v", failed)
	}
}

func TestJobs_CompleteAndRetry(t *testing.T) {
	d := dbtest.New(t)
	q := jobs.New(d.AppPool)
	ctx := context.Background()
	id := enqueue(t, d, "run_proof", "")

	if err := q.Retry(ctx, id); err == nil {
		t.Fatal("only failed jobs may be retried")
	} else {
		var p *httpx.Problem
		if !errors.As(err, &p) || p.Code != "state_conflict" {
			t.Fatalf("want state_conflict, got %v", err)
		}
	}
	if j, _ := q.Claim(ctx, "w", []string{"run_proof"}, time.Minute); j == nil {
		t.Fatal("claim")
	}
	if err := q.Complete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if j, _ := q.Claim(ctx, "w", []string{"run_proof"}, time.Minute); j != nil {
		t.Fatal("a completed job must not be claimed again")
	}
	if err := q.Retry(ctx, "job_missing"); err == nil {
		t.Fatal("retrying an unknown job must fail")
	} else {
		var p *httpx.Problem
		if !errors.As(err, &p) || p.Status != 404 {
			t.Fatalf("want 404, got %v", err)
		}
	}

	failedID := enqueue(t, d, "run_proof", "")
	for i := 0; i < 3; i++ {
		pastRunAfter(t, d, failedID)
		if j, _ := q.Claim(ctx, "w", []string{"run_proof"}, time.Minute); j == nil {
			t.Fatalf("claim %d", i+1)
		}
		if _, err := q.Fail(ctx, failedID, errors.New("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.Retry(ctx, failedID); err != nil {
		t.Fatalf("retry of a failed job: %v", err)
	}
	j, err := q.Claim(ctx, "w", []string{"run_proof"}, time.Minute)
	if err != nil || j == nil || j.Attempts != 1 {
		t.Fatalf("a retried job starts counting from zero: %+v %v", j, err)
	}
}

func TestJobs_TxVariantsShareTheCallersTransaction(t *testing.T) {
	d := dbtest.New(t)
	q := jobs.New(d.AppPool)
	ctx := context.Background()
	id := enqueue(t, d, "run_proof", "")
	if j, _ := q.Claim(ctx, "w", []string{"run_proof"}, time.Minute); j == nil {
		t.Fatal("claim")
	}

	// Rolled back: the job must still be leased, not done.
	rollback := errors.New("work failed")
	err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := jobs.CompleteTx(ctx, tx, id); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var state string
	if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT state FROM jobs WHERE id = $1`, id).Scan(&state)
	}); err != nil || state != "leased" {
		t.Fatalf("a rolled-back CompleteTx must not complete the job: %s %v", state, err)
	}

	err = d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		j, err := jobs.GetTx(ctx, tx, id)
		if err != nil || j.Kind != "run_proof" || j.State != "leased" || j.Attempts != 1 {
			t.Errorf("GetTx: %+v %v", j, err)
		}
		final, err := jobs.FailTx(ctx, tx, id, errors.New("boom"))
		if err != nil || final {
			t.Errorf("first failure is not final: %v %v", final, err)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := jobs.GetTx(ctx, tx, "job_missing")
		return err
	}); err == nil {
		t.Fatal("GetTx of an unknown job must fail")
	}
}
