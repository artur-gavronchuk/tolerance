package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/fixtures/tasks"
	"tolerance/internal/attempts"
	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/jobs"
)

type Service struct {
	pool          *db.Pool
	attempts      *attempts.Service
	webURL        string
	allowLoopback bool
}

// NewService takes the public web origin (for result links) and whether
// loopback preview URLs are accepted (local development only).
func NewService(pool *db.Pool, at *attempts.Service, webURL string, allowLoopback bool) *Service {
	return &Service{pool: pool, attempts: at, webURL: strings.TrimRight(webURL, "/"), allowLoopback: allowLoopback}
}

func conflict(code, message string) *httpx.Problem {
	return httpx.New(http.StatusConflict, code, message)
}

// Submit records the result of a running attempt and queues its checks, in
// one transaction: either the submission, the closed attempt and the check
// job all exist, or none do.
func (s *Service) Submit(ctx context.Context, actor identity.Actor, attemptID string, in Input) (Submitted, error) {
	if actor.Kind != identity.KindAgent || actor.AgentID == "" {
		return Submitted{}, httpx.Forbidden("An agent API key is required")
	}
	if err := in.Validate(s.allowLoopback); err != nil {
		return Submitted{}, err
	}
	var cost []byte
	if in.Cost != nil {
		cost, _ = json.Marshal(in.Cost)
	}
	subID := idgen.New("sub")

	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var compID, kind, status string
		var no int
		var snapshot []byte
		var voided bool
		err := tx.QueryRow(ctx, `SELECT competition_id, kind, attempt_no, status, agent_snapshot, voided_at IS NOT NULL
			FROM attempts WHERE id = $1 AND agent_id = $2 FOR UPDATE`, attemptID, actor.AgentID).
			Scan(&compID, &kind, &no, &status, &snapshot, &voided)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		if status == attempts.StatusSubmitted {
			return conflict("already_submitted", "This attempt already has a submission")
		}
		if status != attempts.StatusRunning || voided {
			return conflict("attempt_not_running", "This attempt is no longer running")
		}

		var compStatus, suite string
		var deadline time.Time
		if err := tx.QueryRow(ctx, `SELECT status, deadline, coalesce(check_suite, '') FROM competitions WHERE id = $1 FOR SHARE`, compID).
			Scan(&compStatus, &deadline, &suite); err != nil {
			return err
		}
		if compStatus != "active" || !time.Now().Before(deadline) {
			return conflict("deadline_passed", "The competition is closed or its deadline has passed")
		}
		if suite == "" {
			return httpx.StateConflict("This competition does not take connector submissions")
		}
		bundle, err := tasks.Load(suite)
		if err != nil {
			return err
		}

		commitLink := "not_provided"
		if in.CommitSHA != "" {
			commitLink = "unverified"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO submissions (id, competition_id, agent_id, source, artifact, summary, preview_url, repo_url,
				commit_sha, notes, reported_cost, attempt_id, attempt_kind, attempt_no, agent_snapshot, commit_link)
			VALUES ($1, $2, $3, 'manual', 'app', $4, $5, NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), $9, $10, $11, $12, $13, $14)`,
			subID, compID, actor.AgentID, in.Summary, in.PreviewURL, in.RepoURL, in.CommitSHA, in.Notes, cost,
			attemptID, kind, no, snapshot, commitLink); err != nil {
			return err
		}
		if err := s.attempts.MarkSubmitted(ctx, tx, attemptID); err != nil {
			return err
		}
		if err := s.attempts.RecordSystemEvent(ctx, tx, attemptID, "submitted", map[string]any{"submission_id": subID}); err != nil {
			return err
		}

		runID := idgen.New("run")
		if _, err := tx.Exec(ctx, `INSERT INTO check_runs (id, submission_id, suite, suite_version) VALUES ($1, $2, $3, $4)`,
			runID, subID, bundle.Suite, bundle.SuiteVersion); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE submissions SET check_run_id = $2 WHERE id = $1`, subID, runID); err != nil {
			return err
		}
		if _, err := jobs.Enqueue(ctx, tx, "check_submission", map[string]string{"submission_id": subID, "check_run_id": runID}, "check:"+runID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actor.ID, ActorKind: actor.Kind, Action: "submission.created",
			AggregateKind: "submission", AggregateID: subID, RequestID: httpx.RequestID(ctx),
			Payload: map[string]any{"attempt_id": attemptID, "attempt_kind": kind, "attempt_no": no}})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		(pgErr.ConstraintName == "submissions_one_official" || pgErr.ConstraintName == "submissions_one_per_attempt") {
		return Submitted{}, conflict("already_submitted", "A submission already exists for this attempt")
	}
	if err != nil {
		return Submitted{}, err
	}
	v, err := s.Get(ctx, subID)
	if err != nil {
		return Submitted{}, err
	}
	return Submitted{View: v, ResultURL: s.webURL + "/submissions/" + subID}, nil
}

const viewSelect = `SELECT s.id, s.competition_id, c.slug, a.name, u.handle, s.source, s.match_id, s.submitted_at, s.artifact, s.summary,
	s.preview_url, s.repo_url, s.preview_kind, s.preview_body, s.score_status, s.total, s.scores, s.points_awarded, s.judged_at,
	r.rank, r.rank_of,
	s.attempt_id, s.attempt_kind, s.attempt_no, att.started_at, s.agent_snapshot, s.commit_sha, s.commit_link, s.verification, s.notes,
	s.reported_cost, s.unscored_reason,
	cr.id, cr.status, cr.suite, cr.suite_version, cr.functional_score, cr.finished_at,
	c.criteria,
	EXISTS (SELECT 1 FROM check_results x WHERE x.check_run_id = s.check_run_id AND x.override_status IS NOT NULL),
	EXISTS (SELECT 1 FROM judgments j WHERE j.submission_id = s.id AND j.kind = 'human' AND j.status = 'completed')
FROM submissions s
JOIN competitions c ON c.id = s.competition_id
JOIN agents a ON a.id = s.agent_id
JOIN users u ON u.id = a.owner_user_id
LEFT JOIN attempts att ON att.id = s.attempt_id
LEFT JOIN check_runs cr ON cr.id = s.check_run_id
LEFT JOIN competition_rankings r ON r.submission_id = s.id `

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func scanView(row pgx.Row, v *View) error {
	var (
		previewKind, previewBody               *string
		scores, snapshot, cost, criteria       []byte
		attemptID                              *string
		attemptStarted                         *time.Time
		runID, runStatus, runSuite, runVersion *string
		runScore                               *int
		runFinished                            *time.Time
		overridden, human                      bool
	)
	if err := row.Scan(&v.ID, &v.CompetitionID, &v.CompetitionSlug, &v.Agent, &v.Author, &v.Source, &v.MatchID, &v.SubmittedAt,
		&v.Artifact, &v.Summary, &v.PreviewURL, &v.RepoURL, &previewKind, &previewBody, &v.ScoreStatus, &v.Total, &scores,
		&v.PointsAwarded, &v.JudgedAt, &v.Rank, &v.RankOf,
		&attemptID, &v.Attempt.Kind, &v.Attempt.No, &attemptStarted, &snapshot, &v.CommitSHA, &v.CommitLink, &v.Verification, &v.Notes,
		&cost, &v.UnscoredReason,
		&runID, &runStatus, &runSuite, &runVersion, &runScore, &runFinished,
		&criteria, &overridden, &human); err != nil {
		return err
	}
	v.SubmittedAt = v.SubmittedAt.UTC()
	v.JudgedAt = utc(v.JudgedAt)
	if previewKind != nil && previewBody != nil {
		v.Preview = &Preview{Kind: *previewKind, Body: *previewBody}
	}
	v.Attempt.ID = attemptID
	v.Attempt.StartedAt = utc(attemptStarted)
	if attemptStarted != nil {
		d := int(v.SubmittedAt.Sub(*attemptStarted).Seconds())
		if d < 0 {
			d = 0
		}
		v.Attempt.DurationSeconds = &d
	}
	if len(snapshot) > 0 {
		v.AgentSnapshot = snapshot
	}
	if len(cost) > 0 {
		v.ReportedCost = cost
	}
	if runID != nil {
		v.CheckRun = &CheckRunInfo{ID: *runID, Status: *runStatus, Suite: *runSuite, SuiteVersion: *runVersion,
			FunctionalScore: runScore, FinishedAt: utc(runFinished)}
	}
	var err error
	if v.Scores, err = enrichScores(scores, criteria); err != nil {
		return err
	}
	v.Limitations = limitations(v.CommitLink, v.PreviewURL != nil, v.Scores, overridden || human)
	return nil
}

// enrichScores decodes stored scores and fills what older rows lack (weight
// and source from the competition's criteria, status from the score).
func enrichScores(scores, criteria []byte) ([]ScoreEntry, error) {
	if len(scores) == 0 {
		return nil, nil
	}
	var entries []ScoreEntry
	if err := json.Unmarshal(scores, &entries); err != nil {
		return nil, err
	}
	var crit []struct {
		Name   string `json:"name"`
		Weight int    `json:"weight"`
		Source string `json:"source"`
	}
	_ = json.Unmarshal(criteria, &crit)
	for i := range entries {
		e := &entries[i]
		for _, c := range crit {
			if c.Name != e.Name {
				continue
			}
			if e.Weight == 0 {
				e.Weight = c.Weight
			}
			if e.Source == "" {
				e.Source = c.Source
			}
		}
		if e.Source == "" {
			e.Source = "llm"
		}
		if e.Status == "" {
			e.Status = "not_rated"
			if e.Score != nil {
				e.Status = "rated"
			}
		}
	}
	return entries, nil
}

// limitations lists, as codes, what this result does not prove. Two are
// always present for a connector submission: the run is self-reported and
// the preview URL is mutable.
func limitations(commitLink string, hasPreview bool, scores []ScoreEntry, humanReviewed bool) []string {
	out := []string{"self_reported_run"}
	if hasPreview {
		out = append(out, "mutable_preview_url")
	}
	switch commitLink {
	case "not_provided":
		out = append(out, "commit_not_provided")
	case "unverified":
		out = append(out, "commit_unverified")
	case "declared_match":
		out = append(out, "commit_declared_only")
	case "mismatch":
		out = append(out, "commit_mismatch")
	}
	var judgeOff, noSource bool
	for _, e := range scores {
		switch e.Reason {
		case "judge_disabled":
			judgeOff = true
		case "source_not_available":
			noSource = true
		}
	}
	if judgeOff {
		out = append(out, "llm_judge_disabled")
	}
	if noSource {
		out = append(out, "source_not_available")
	}
	if humanReviewed {
		out = append(out, "manual_override_applied")
	}
	return out
}

func (s *Service) query(ctx context.Context, where, order string, args ...any) ([]View, error) {
	out := []View{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, viewSelect+where+" "+order, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v View
			if err := scanView(rows, &v); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}

// Get returns one submission of any status.
func (s *Service) Get(ctx context.Context, id string) (View, error) {
	list, err := s.query(ctx, `WHERE s.id = $1`, ``, id)
	if err != nil {
		return View{}, err
	}
	if len(list) == 0 {
		return View{}, httpx.NotFound()
	}
	return list[0], nil
}

// ListOptions filters public lists. Kind is official (default), practice
// or all; unscored submissions are included only on request.
type ListOptions struct {
	Kind            string
	IncludeUnscored bool
}

func (o ListOptions) filter(firstArg int) (string, []any, error) {
	kind := o.Kind
	if kind == "" {
		kind = attempts.KindOfficial
	}
	var sb strings.Builder
	var args []any
	switch kind {
	case attempts.KindOfficial, attempts.KindPractice:
		sb.WriteString(" AND s.attempt_kind = $" + strconv.Itoa(firstArg))
		args = append(args, kind)
	case "all":
	default:
		return "", nil, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "kind must be official, practice or all", "kind", "invalid")
	}
	if !o.IncludeUnscored {
		sb.WriteString(" AND s.score_status = 'scored'")
	}
	return sb.String(), args, nil
}

// ListForCompetition returns a competition's submissions: scored ones best
// first, then (on request) the rest, newest first.
func (s *Service) ListForCompetition(ctx context.Context, slug string, o ListOptions) ([]View, error) {
	var exists bool
	if err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM competitions WHERE slug = $1 AND status <> 'draft')`, slug).Scan(&exists)
	}); err != nil {
		return nil, err
	}
	if !exists {
		return nil, httpx.NotFound()
	}
	filter, args, err := o.filter(2)
	if err != nil {
		return nil, err
	}
	return s.query(ctx, `WHERE c.slug = $1`+filter, `ORDER BY (s.score_status = 'scored') DESC,
		CASE WHEN s.score_status = 'scored' THEN s.total END DESC NULLS LAST,
		CASE WHEN s.score_status = 'scored' THEN s.submitted_at END ASC,
		s.submitted_at DESC`, append([]any{slug}, args...)...)
}

// ListForAgent returns an agent's submissions, newest first. The agent is
// looked up by name, ignoring case.
func (s *Service) ListForAgent(ctx context.Context, name string, o ListOptions) ([]View, error) {
	var exists bool
	if err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM agents WHERE lower(name) = lower($1))`, name).Scan(&exists)
	}); err != nil {
		return nil, err
	}
	if !exists {
		return nil, httpx.NotFound()
	}
	filter, args, err := o.filter(2)
	if err != nil {
		return nil, err
	}
	return s.query(ctx, `WHERE lower(a.name) = lower($1)`+filter, `ORDER BY s.submitted_at DESC`, append([]any{name}, args...)...)
}

// ListForOwner returns everything the user's agent has submitted, of any
// kind and status, newest first.
func (s *Service) ListForOwner(ctx context.Context, userID string) ([]View, error) {
	return s.query(ctx, `WHERE a.owner_user_id = $1`, `ORDER BY s.submitted_at DESC`, userID)
}
