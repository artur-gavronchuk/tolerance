package checks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/fixtures/tasks"
	"tolerance/internal/attempts"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/submissions"
)

const (
	checkJobKind = "check_submission"
	judgeJobKind = "judge_submission"
	claimLease   = 5 * time.Minute
)

type Options struct {
	// JudgeEnabled queues a qualitative judging job after a run completes
	// instead of finalising the score at once.
	JudgeEnabled bool
}

type Service struct {
	pool     *db.Pool
	queue    *jobs.Queue
	subs     *submissions.Service
	attempts *attempts.Service
	opts     Options
}

func NewService(pool *db.Pool, queue *jobs.Queue, subs *submissions.Service, at *attempts.Service, opts Options) *Service {
	return &Service{pool: pool, queue: queue, subs: subs, attempts: at, opts: opts}
}

func conflict(code, message string) *httpx.Problem {
	return httpx.New(http.StatusConflict, code, message)
}

// jobPayload is what Submit stores on a check job.
type jobPayload struct {
	SubmissionID string `json:"submission_id"`
	CheckRunID   string `json:"check_run_id"`
}

// Claim hands the next queued check run to a checker worker, or returns nil
// when there is nothing to do. Jobs that turn out to be stale (their run
// was superseded or already finished) are closed and skipped.
func (s *Service) Claim(ctx context.Context, workerID string) (*ClaimResponse, error) {
	for i := 0; i < 10; i++ {
		job, err := s.queue.Claim(ctx, workerID, []string{checkJobKind}, claimLease)
		if err != nil || job == nil {
			return nil, err
		}
		resp, err := s.start(ctx, job)
		if err != nil || resp != nil {
			return resp, err
		}
	}
	return nil, nil
}

func (s *Service) start(ctx context.Context, job *jobs.Job) (*ClaimResponse, error) {
	var p jobPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return nil, err
	}
	var resp *ClaimResponse
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var status, suite, version string
		err := tx.QueryRow(ctx, `SELECT status, suite, suite_version FROM check_runs WHERE id = $1 FOR UPDATE`, p.CheckRunID).Scan(&status, &suite, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			return jobs.CompleteTx(ctx, tx, job.ID)
		}
		if err != nil {
			return err
		}
		ref, err := s.subs.Lock(ctx, tx, p.SubmissionID)
		if err != nil {
			return err
		}
		// A finished run, or one replaced by a newer recheck, needs no work.
		// An infra_error run is picked up again when an admin retries the job.
		if (status != RunQueued && status != RunRunning && status != RunInfraError) || ref.CheckRunID == nil || *ref.CheckRunID != p.CheckRunID {
			return jobs.CompleteTx(ctx, tx, job.ID)
		}
		if _, err := tx.Exec(ctx, `UPDATE check_runs SET status = 'running', started_at = now(), finished_at = NULL, error = NULL WHERE id = $1`, p.CheckRunID); err != nil {
			return err
		}
		if err := s.subs.SetChecking(ctx, tx, ref.ID); err != nil {
			return err
		}
		if ref.AttemptID != nil {
			if err := s.attempts.RecordSystemEvent(ctx, tx, *ref.AttemptID, "check_started", nil); err != nil {
				return err
			}
		}
		resp = &ClaimResponse{JobID: job.ID, CheckRunID: p.CheckRunID, SubmissionID: ref.ID, PreviewURL: ref.PreviewURL,
			Suite: suite, SuiteVersion: version, CommitSHA: ref.CommitSHA, RepoURL: ref.RepoURL}
		return nil
	})
	return resp, err
}

// lockedRun is a check run held for the rest of a transaction.
type lockedRun struct {
	payload jobPayload
	suite   string
}

// lockRun locks the job and its run and confirms the report is still
// expected: the job is leased and the run is running.
func (s *Service) lockRun(ctx context.Context, tx pgx.Tx, jobID string) (lockedRun, error) {
	job, err := jobs.GetTx(ctx, tx, jobID)
	if err != nil {
		return lockedRun{}, err
	}
	if job.Kind != checkJobKind || job.State != "leased" {
		return lockedRun{}, conflict("state_conflict", "This job is not waiting for a report")
	}
	var lr lockedRun
	if err := json.Unmarshal(job.Payload, &lr.payload); err != nil {
		return lockedRun{}, err
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status, suite FROM check_runs WHERE id = $1 FOR UPDATE`, lr.payload.CheckRunID).Scan(&status, &lr.suite); err != nil {
		return lockedRun{}, err
	}
	if status != RunRunning {
		return lockedRun{}, conflict("state_conflict", "This check run is not running")
	}
	return lr, nil
}

// Complete stores the checker's report for a claimed job and settles the
// submission accordingly:
//
//   - completed: results and evidence are stored, the score is computed;
//   - unreachable: the app could not be opened; the submission becomes
//     unverifiable (never scored from its description);
//   - infra_error: the checker itself failed; the job is retried and, after
//     the last attempt, the submission is marked failed (a platform error,
//     never a loss for the participant).
func (s *Service) Complete(ctx context.Context, jobID string, in CompleteInput) error {
	var lr lockedRun
	if err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		lr, err = s.lockRun(ctx, tx, jobID)
		return err
	}); err != nil {
		return err
	}
	bundle, err := tasks.Load(lr.suite)
	if err != nil {
		return err
	}

	evidence, verr := in.validate(bundle)
	if verr != nil {
		// A malformed report is the checker's fault, so it is handled as an
		// infrastructure failure and never counted against the participant.
		if ferr := s.failRun(ctx, jobID, "the checker sent an invalid report: "+verr.Error()); ferr != nil {
			return ferr
		}
		return verr
	}
	if in.Status == RunCompleted {
		for _, r := range in.Results {
			if r.Status == StatusInfraError {
				msg := "the checker reported an infrastructure error in check " + r.CheckID
				if err := s.failRun(ctx, jobID, msg); err != nil {
					return err
				}
				return nil
			}
		}
	}
	if in.Status == RunInfraError {
		msg := truncate(strings.TrimSpace(in.Error), maxTextField)
		if msg == "" {
			msg = "the checker reported an infrastructure error"
		}
		return s.failRun(ctx, jobID, msg)
	}

	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		lr, err := s.lockRun(ctx, tx, jobID)
		if err != nil {
			return err
		}
		ref, err := s.subs.Lock(ctx, tx, lr.payload.SubmissionID)
		if err != nil {
			return err
		}
		if ref.CheckRunID == nil || *ref.CheckRunID != lr.payload.CheckRunID {
			// A recheck replaced this run while it was in flight.
			return jobs.CompleteTx(ctx, tx, jobID)
		}
		runID := lr.payload.CheckRunID
		if err := s.storeEvidence(ctx, tx, runID, evidence); err != nil {
			return err
		}

		if in.Status == RunUnreachable {
			return s.settleUnreachable(ctx, tx, jobID, runID, ref, bundle, in)
		}
		return s.settleCompleted(ctx, tx, jobID, runID, ref, bundle, in)
	})
}

func (s *Service) storeEvidence(ctx context.Context, tx pgx.Tx, runID string, evidence []decodedEvidence) error {
	for _, e := range evidence {
		sum := sha256.Sum256(e.Bytes)
		if _, err := tx.Exec(ctx, `INSERT INTO evidence_blobs (id, check_run_id, check_id, kind, label, content_type, bytes, size, sha256)
			VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9)`,
			idgen.New("evd"), runID, e.CheckID, e.Kind, e.Label, e.ContentType, e.Bytes, len(e.Bytes), hex.EncodeToString(sum[:])); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) settleUnreachable(ctx context.Context, tx pgx.Tx, jobID, runID string, ref submissions.Ref, b tasks.Bundle, in CompleteInput) error {
	reason := truncate(strings.TrimSpace(in.UnreachableReason), maxReason)
	if reason == "" {
		reason = "the app could not be opened"
	}
	for _, c := range b.Checks {
		if _, err := tx.Exec(ctx, `INSERT INTO check_results (id, check_run_id, check_id, title, requirement, required, weight, status, expected, actual)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'insufficient_data', $8, $9)`,
			idgen.New("res"), runID, c.ID, c.Title, c.Requirement, c.Required, c.Weight,
			"The app opens, so this requirement can be observed", "Not observed: "+reason); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE check_runs SET status = 'unreachable', unreachable_reason = $2, checker_version = $3, browser = $4,
			functional_score = NULL, error = NULL, finished_at = now() WHERE id = $1`,
		runID, reason, truncate(in.CheckerVersion, 80), truncate(in.Browser, 80)); err != nil {
		return err
	}
	if err := s.subs.MarkUnverifiable(ctx, tx, ref.ID, reason); err != nil {
		return err
	}
	if err := jobs.CompleteTx(ctx, tx, jobID); err != nil {
		return err
	}
	if ref.AttemptID == nil {
		return nil
	}
	if err := s.attempts.RecordSystemEvent(ctx, tx, *ref.AttemptID, "check_finished", map[string]any{"status": "unreachable"}); err != nil {
		return err
	}
	return s.attempts.RecordSystemEvent(ctx, tx, *ref.AttemptID, "result", map[string]any{"status": "unverifiable", "reason": reason})
}

func (s *Service) settleCompleted(ctx context.Context, tx pgx.Tx, jobID, runID string, ref submissions.Ref, b tasks.Bundle, in CompleteInput) error {
	defs := map[string]tasks.CheckDef{}
	for _, c := range b.Checks {
		defs[c.ID] = c
	}
	scoring := make([]Result, 0, len(in.Results))
	for _, r := range in.Results {
		d := defs[r.CheckID] // present: validate() guaranteed it
		diag := []byte(r.Diagnostics)
		if len(diag) == 0 {
			diag = []byte("{}")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO check_results (id, check_run_id, check_id, title, requirement, required, weight, status, expected, actual, diagnostics, duration_ms)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			idgen.New("res"), runID, d.ID, d.Title, d.Requirement, d.Required, d.Weight, r.Status,
			truncate(r.Expected, maxTextField), truncate(r.Actual, maxTextField), diag, r.DurationMS); err != nil {
			return err
		}
		scoring = append(scoring, Result{Status: r.Status, Weight: d.Weight})
	}

	var buildInfo []byte
	if in.BuildInfo != nil {
		buildInfo, _ = json.Marshal(BuildInfo{Commit: truncate(strings.TrimSpace(in.BuildInfo.Commit), 64), Found: in.BuildInfo.Found})
	}
	if _, err := tx.Exec(ctx, `UPDATE check_runs SET status = 'completed', checker_version = $2, browser = $3, functional_score = $4,
			build_info = $5, unreachable_reason = NULL, error = NULL, finished_at = now() WHERE id = $1`,
		runID, truncate(in.CheckerVersion, 80), truncate(in.Browser, 80), FunctionalScore(scoring), buildInfo); err != nil {
		return err
	}
	if link := commitLink(in.BuildInfo, ref.CommitSHA); link != "" {
		if err := s.subs.SetCommitLink(ctx, tx, ref.ID, link); err != nil {
			return err
		}
	}
	if err := jobs.CompleteTx(ctx, tx, jobID); err != nil {
		return err
	}
	if ref.AttemptID != nil {
		if err := s.attempts.RecordSystemEvent(ctx, tx, *ref.AttemptID, "check_finished",
			map[string]any{"status": "completed", "functional_score": FunctionalScore(scoring)}); err != nil {
			return err
		}
	}
	if s.opts.JudgeEnabled {
		_, err := jobs.Enqueue(ctx, tx, judgeJobKind, map[string]string{"submission_id": ref.ID, "check_run_id": runID}, "judge:"+runID)
		return err
	}
	return s.Finalize(ctx, tx, ref.ID)
}

// commitLink compares the commit a deployment declares with the submitted
// one. It returns "" when nothing can be said, in which case the link stays
// as it was ("unverified" or "not_provided"). A declaration is not proof,
// so there is no verified outcome.
func commitLink(declared *BuildInfo, sha string) string {
	if sha == "" || declared == nil || !declared.Found {
		return ""
	}
	d := strings.ToLower(strings.TrimSpace(declared.Commit))
	if d == "" {
		return ""
	}
	if d == sha || (len(d) >= 7 && len(d) < len(sha) && strings.HasPrefix(sha, d)) {
		return "declared_match"
	}
	return "mismatch"
}

// failRun handles an infrastructure failure of a run: the job is retried
// with backoff and, after the last attempt, the submission is failed.
func (s *Service) failRun(ctx context.Context, jobID, msg string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		lr, err := s.lockRun(ctx, tx, jobID)
		if err != nil {
			return err
		}
		return s.infraFailure(ctx, tx, jobID, lr.payload, msg)
	})
}

func (s *Service) infraFailure(ctx context.Context, tx pgx.Tx, jobID string, p jobPayload, msg string) error {
	msg = truncate(msg, maxTextField)
	final, err := jobs.FailTx(ctx, tx, jobID, errors.New(msg))
	if err != nil {
		return err
	}
	if !final {
		_, err := tx.Exec(ctx, `UPDATE check_runs SET status = 'queued', error = $2 WHERE id = $1`, p.CheckRunID, msg)
		return err
	}
	return s.giveUp(ctx, tx, p, msg)
}

// giveUp ends a run that will not be retried: the run is an infrastructure
// error and the submission is failed, which is a platform error and never
// a result.
func (s *Service) giveUp(ctx context.Context, tx pgx.Tx, p jobPayload, msg string) error {
	if _, err := tx.Exec(ctx, `UPDATE check_runs SET status = 'infra_error', error = $2, finished_at = now() WHERE id = $1`, p.CheckRunID, msg); err != nil {
		return err
	}
	ref, err := s.subs.Lock(ctx, tx, p.SubmissionID)
	if err != nil {
		return err
	}
	if ref.CheckRunID == nil || *ref.CheckRunID != p.CheckRunID {
		return nil
	}
	if err := s.subs.MarkFailed(ctx, tx, ref.ID, "checker infrastructure error"); err != nil {
		return err
	}
	if ref.AttemptID == nil {
		return nil
	}
	return s.attempts.RecordSystemEvent(ctx, tx, *ref.AttemptID, "result", map[string]any{"status": "failed", "reason": "platform_error"})
}

// Reap returns jobs whose worker vanished to the queue and fails the
// submissions whose check job ran out of attempts that way.
func (s *Service) Reap(ctx context.Context) (int, error) {
	n, err := s.queue.Reclaim(ctx)
	if err != nil {
		return n, err
	}
	var stuck []jobPayload
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT j.payload->>'submission_id', j.payload->>'check_run_id'
			FROM jobs j JOIN check_runs r ON r.id = j.payload->>'check_run_id'
			WHERE j.kind = $1 AND j.state = 'failed' AND r.status IN ('queued', 'running')`, checkJobKind)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p jobPayload
			if err := rows.Scan(&p.SubmissionID, &p.CheckRunID); err != nil {
				return err
			}
			stuck = append(stuck, p)
		}
		return rows.Err()
	})
	if err != nil {
		return n, err
	}
	for _, p := range stuck {
		if err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return s.giveUp(ctx, tx, p, "the checker lease expired on every attempt")
		}); err != nil {
			return n, err
		}
	}
	return n + len(stuck), nil
}
