package qualifications

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/agents"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/limits"
	"tolerance/internal/proofs"
	"tolerance/internal/skills"
)

type Service struct {
	pool    *db.Pool
	proofs  *proofs.Service
	rnd     *rand.Rand
	minPool int
}

func NewService(pool *db.Pool, ps *proofs.Service) *Service {
	return &Service{pool: pool, proofs: ps, rnd: rand.New(rand.NewSource(time.Now().UnixNano())), minPool: skills.MinPool}
}

// SetMinPool overrides how many issuable tasks a skill needs before a run may
// start (ARENA_SKILL_MIN_POOL). It exists because the public practice catalog in
// this repository holds three tasks per skill while the private rating catalog
// the production server mounts holds more: a floor that is right for production
// would freeze every skill in a local run. Zero never freezes.
func (s *Service) SetMinPool(n int) { s.minPool = n }

// handOut queues one task of a run and records the exposure in the same
// transaction: the task's repository leaves the platform the moment the proof is
// claimable. Every path that hands a task to an agent goes through here, so the
// count cannot drift from what actually left. Exposure counts distinct agents,
// so the platform's own requeue after an infra error adds nothing.
func (s *Service) handOut(ctx context.Context, tx pgx.Tx, agentID, runID, taskSlug string, position int, requeue bool) (proofs.Proof, error) {
	var (
		p   proofs.Proof
		err error
	)
	if requeue {
		p, err = s.proofs.RequeueQualificationProof(ctx, tx, agentID, runID, taskSlug, position)
	} else {
		p, err = s.proofs.CreateQualificationProof(ctx, tx, agentID, runID, taskSlug, position)
	}
	if err != nil {
		return proofs.Proof{}, err
	}
	if _, err := skills.RecordExposure(ctx, tx, taskSlug, agentID); err != nil {
		return proofs.Proof{}, err
	}
	return p, nil
}

const runCols = `id, agent_id, version_id, skill_slug, status, created_at, finished_at, score, rating_before, rating_after, uncertainty_after, task_slugs`

func scanRun(row interface{ Scan(...any) error }, r *Run) error {
	if err := row.Scan(&r.ID, &r.AgentID, &r.VersionID, &r.SkillSlug, &r.Status, &r.CreatedAt, &r.FinishedAt, &r.Score, &r.RatingBefore, &r.RatingAfter, &r.UncertaintyAfter, &r.TaskSlugs); err != nil {
		return err
	}
	r.CreatedAt = r.CreatedAt.UTC()
	if r.FinishedAt != nil {
		u := r.FinishedAt.UTC()
		r.FinishedAt = &u
	}
	return nil
}

func (s *Service) agentOf(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM agents WHERE owner_user_id = $1`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", httpx.New(http.StatusNotFound, "no_agent", "Create an agent first")
	}
	return id, err
}

// Start opens a run: picks three tasks, queues the first one.
func (s *Service) Start(ctx context.Context, userID, skill string) (Run, error) {
	var run Run
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		agentID, err := s.agentOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT 1 FROM agents WHERE id = $1 FOR UPDATE`, agentID); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM skills WHERE slug = $1)`, skill).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.New(http.StatusNotFound, "unknown_skill", "No such skill")
		}
		if err := proofs.BannedGuard(ctx, tx, agentID); err != nil {
			return err
		}
		var passed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM proofs WHERE agent_id = $1 AND kind = 'proof' AND status = 'passed')`, agentID).Scan(&passed); err != nil {
			return err
		}
		if !passed {
			return httpx.New(http.StatusConflict, "agent_not_operational", "Pass the basic proof first")
		}
		// Same rule as proofs.Create: an offline connector would let the first
		// task expire unclaimed and burn one of the day's runs.
		var online bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM agent_presence WHERE agent_id = $1 AND last_seen_at > now() - make_interval(secs => $2))`, agentID, agents.PresenceTTL.Seconds()).Scan(&online); err != nil {
			return err
		}
		if !online {
			return httpx.New(http.StatusConflict, "agent_offline", "The connector is not online; run `arena connect` first")
		}
		var versionID *string
		if err := tx.QueryRow(ctx, `SELECT current_version_id FROM agents WHERE id = $1`, agentID).Scan(&versionID); err != nil {
			return err
		}
		if versionID == nil {
			return httpx.New(http.StatusConflict, "no_version", "The connector has not reported the agent version yet; update it and run `arena connect`")
		}
		var open bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM qualification_runs WHERE agent_id = $1 AND status = 'running')`, agentID).Scan(&open); err != nil {
			return err
		}
		if open {
			return httpx.New(http.StatusConflict, "qualification_in_progress", "A qualification run is already in progress")
		}
		var today int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM qualification_runs WHERE agent_id = $1 AND skill_slug = $2 AND created_at > now() - interval '24 hours'`, agentID, skill).Scan(&today); err != nil {
			return err
		}
		if today >= limits.Cap(dailyLimit) {
			return httpx.New(http.StatusTooManyRequests, "daily_limit", "At most 3 qualification runs per skill per day")
		}
		pool, err := skills.Pool(ctx, tx, skill)
		if err != nil {
			return err
		}
		// Two floors, one answer. A run cannot be built out of fewer than
		// tasksPerRun tasks at all, and below the configured policy floor the pool
		// is too worn to rate on. Both mean "not now, the skill is being
		// refilled", so they share one code instead of two.
		if skills.Frozen(len(pool), max(s.minPool, tasksPerRun)) {
			return skills.ErrFrozen()
		}
		var recent []string
		if err := tx.QueryRow(ctx, `SELECT coalesce(array_agg(t), '{}') FROM (SELECT unnest(task_slugs) t FROM
			(SELECT task_slugs FROM qualification_runs WHERE agent_id = $1 AND skill_slug = $2 ORDER BY created_at DESC LIMIT $3) r) x`, agentID, skill, recentRuns).Scan(&recent); err != nil {
			return err
		}
		picked := skills.Pick(pool, recent, tasksPerRun, s.rnd)
		if err := scanRun(tx.QueryRow(ctx, `INSERT INTO qualification_runs (id, agent_id, version_id, skill_slug, task_slugs) VALUES ($1, $2, $3, $4, $5) RETURNING `+runCols,
			idgen.New("qrun"), agentID, *versionID, skill, picked), &run); err != nil {
			return err
		}
		first, err := s.handOut(ctx, tx, agentID, run.ID, picked[0], 1, false)
		if err != nil {
			return err
		}
		run.Tasks = []proofs.Proof{first}
		return audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "qualification.started", AggregateKind: "qualification_run", AggregateID: run.ID,
			Payload: map[string]any{"skill": skill, "tasks": picked}, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && (strings.Contains(pgErr.ConstraintName, "qualification_runs_one_open_idx") || strings.Contains(pgErr.ConstraintName, "proofs_one_open_idx")) {
		return Run{}, httpx.New(http.StatusConflict, "qualification_in_progress", "A qualification or proof is already in progress")
	}
	return run, err
}

func (s *Service) tasksOf(ctx context.Context, tx pgx.Tx, runID string) ([]proofs.Proof, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM proofs WHERE qualification_run_id = $1 ORDER BY position, created_at`, runID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := make([]proofs.Proof, 0, len(ids))
	for _, id := range ids {
		p, err := s.proofs.GetByIDTx(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, s.proofs.MaskHidden(p))
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, userID, id string) (Run, error) {
	var run Run
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanRun(tx.QueryRow(ctx, `SELECT `+runCols+` FROM qualification_runs WHERE id = $1 AND agent_id = (SELECT id FROM agents WHERE owner_user_id = $2)`, id, userID), &run); err != nil {
			return err
		}
		var err error
		run.Tasks, err = s.tasksOf(ctx, tx, run.ID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, httpx.NotFound()
	}
	return run, err
}

func (s *Service) List(ctx context.Context, userID string) ([]Run, error) {
	out := []Run{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		agentID, err := s.agentOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+runCols+` FROM qualification_runs WHERE agent_id = $1 ORDER BY created_at DESC LIMIT 50`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Run
			if err := scanRun(rows, &r); err != nil {
				return err
			}
			r.Tasks = []proofs.Proof{}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// Abort ends a running run without touching the rating and expires its open task. A run that is
// already scored or aborted is left alone.
func (s *Service) Abort(ctx context.Context, runID, reason string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return s.abortTx(ctx, tx, runID, reason)
	})
}

// abortTx is Abort inside the caller's transaction. Its UPDATE takes the run row lock, so it serializes
// with OnProofFinished (which locks the same row), and it touches the proofs only when it really ended the run.
func (s *Service) abortTx(ctx context.Context, tx pgx.Tx, runID, reason string) error {
	tag, err := tx.Exec(ctx, `UPDATE qualification_runs SET status = 'aborted', finished_at = now() WHERE id = $1 AND status = 'running'`, runID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE proofs SET status = 'expired', finished_at = now(), failure_reason = $2
		WHERE qualification_run_id = $1 AND status IN ('queued', 'claimed', 'running_agent', 'diff_submitted', 'running_sandbox')`, runID, "run_aborted: "+reason)
	return err
}
