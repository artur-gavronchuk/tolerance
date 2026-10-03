package submissions

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/daily"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/platform/limits"
	"tolerance/internal/platform/sanitize"
)

type Service struct {
	pool  *db.Pool
	daily *daily.Service
}

func NewService(pool *db.Pool, d *daily.Service) *Service { return &Service{pool: pool, daily: d} }

func invalid(msg string) error {
	return httpx.WithField(http.StatusUnprocessableEntity, "invalid_upload", msg, "file", "invalid")
}

// Create validates an upload, stores it as a queued submission and enqueues its run. An upload for the
// task of the current day counts for the day (attempt-limited); any other task is practice (day = null).
func (s *Service) Create(ctx context.Context, userID, taskSlug, filename string, data []byte, madeWith string) (Submission, error) {
	today := daily.Today()
	todaySlug, err := s.daily.TaskFor(ctx, today)
	if err != nil {
		return Submission{}, err
	}
	if taskSlug == "" {
		taskSlug = todaySlug
	}
	var repoTar []byte
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT repo_tar FROM tasks WHERE slug = $1`, taskSlug).Scan(&repoTar)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Submission{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_task", "Unknown task", "task_slug", "invalid")
	}
	if err != nil {
		return Submission{}, err
	}

	var diff string
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".zip":
		diff, err = ZipToDiff(ctx, repoTar, data)
		if err != nil {
			return Submission{}, err
		}
	case ".patch", ".diff":
		if !utf8.Valid(data) {
			return Submission{}, invalid("The patch must be UTF-8 text")
		}
		diff = string(data)
	default:
		return Submission{}, invalid("Upload a .zip of the edited repository or a .patch / .diff file")
	}
	if strings.TrimSpace(diff) == "" {
		return Submission{}, invalid("The upload has no changes compared to the task's repository")
	}
	if len(diff) > maxDiffBytes {
		return Submission{}, invalid("The resulting diff is larger than 1 MiB")
	}
	madeWith = sanitize.CleanText(madeWith, maxMadeWith)

	var day *string
	if taskSlug == todaySlug {
		day = &today
	}
	id := idgen.New("sub")
	var out Submission
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if day != nil {
			// Serialize a person's uploads so two parallel requests cannot both take the last attempt.
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('submissions:' || $1))`, userID); err != nil {
				return err
			}
			var used int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM submissions WHERE user_id = $1 AND day = $2::date AND status <> 'infra_error'`,
				userID, *day).Scan(&used); err != nil {
				return err
			}
			if used >= limits.Cap(daily.AttemptsPerDay) {
				return httpx.New(http.StatusTooManyRequests, "attempts_exhausted", "All attempts for today's task are used; a new task opens at UTC midnight")
			}
		}
		out, err = scan(tx.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO submissions (id, user_id, task_slug, day, diff, made_with, total_tests)
				SELECT $1, $2, t.slug, $4::date, $5, $6, t.hidden_tests FROM tasks t WHERE t.slug = $3
				RETURNING *)
			SELECT `+cols+` FROM ins s`, id, userID, taskSlug, day, diff, madeWith))
		if err != nil {
			return err
		}
		if _, err := jobs.Enqueue(ctx, tx, JobKind, RunPayload{SubmissionID: id}, ""); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "submission.created", AggregateKind: "submission", AggregateID: id,
			RequestID: httpx.RequestID(ctx)})
	})
	return out, err
}

// Get returns one of the caller's own submissions.
func (s *Service) Get(ctx context.Context, userID, id string) (Submission, error) {
	var out Submission
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM submissions s WHERE s.id = $1 AND s.user_id = $2`, id, userID))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Submission{}, httpx.NotFound()
	}
	return out, err
}

// My is the daily response's "my" block for one day.
type My struct {
	AttemptsUsed int          `json:"attempts_used"`
	Best         *Submission  `json:"best"`
	Submissions  []Submission `json:"submissions"`
}

// MyDay builds the signed-in person's block for a day; it satisfies daily.MyFunc.
func (s *Service) MyDay(ctx context.Context, userID, day string) (any, error) {
	my := My{Submissions: []Submission{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+cols+` FROM submissions s WHERE s.user_id = $1 AND s.day = $2::date ORDER BY s.created_at DESC`, userID, day)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			sub, err := scan(rows)
			if err != nil {
				return err
			}
			my.Submissions = append(my.Submissions, sub)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	var kind, direction string
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var d *string
		err := tx.QueryRow(ctx, `SELECT t.kind, t.direction FROM daily_tasks dt JOIN tasks t ON t.slug = dt.task_slug WHERE dt.day = $1::date`, day).Scan(&kind, &d)
		if d != nil {
			direction = *d
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	for i := range my.Submissions {
		sub := my.Submissions[i]
		if sub.Status != StatusInfraError {
			my.AttemptsUsed++
		}
		if sub.Status != StatusPassed && sub.Status != StatusFailed {
			continue
		}
		// Submissions are newest first: on a tie the later index is the earlier submission, so prefer it.
		if my.Best == nil || betterOrEqual(kind, direction, sub, *my.Best) {
			b := sub
			my.Best = &b
		}
	}
	return my, nil
}

// betterOrEqual compares a submission with the current best of a day: tests passed for bugfix tasks, the
// score (by direction) for optimize tasks. A min task ranks only fully valid submissions, because an invalid
// case scores 0.
func betterOrEqual(kind, direction string, a, b Submission) bool {
	if kind != "optimize" {
		return a.PassedTests >= b.PassedTests
	}
	if direction == "min" {
		if (a.PassedTests == a.TotalTests) != (b.PassedTests == b.TotalTests) {
			return a.PassedTests == a.TotalTests
		}
	}
	as, bs := 0.0, 0.0
	if a.Score != nil {
		as = *a.Score
	}
	if b.Score != nil {
		bs = *b.Score
	}
	if direction == "min" {
		return as <= bs
	}
	return as >= bs
}
