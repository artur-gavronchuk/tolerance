package proofs

import (
	"context"
	"errors"
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
	"tolerance/internal/platform/jobs"
	"tolerance/internal/platform/sanitize"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

const proofCols = `id, agent_id, task_slug, status, created_at, claimed_at, diff_submitted_at, finished_at,
	diff, agent_log_tail, agent_duration_ms, agent_exit_code, sandbox_result, failure_reason`

func utcp(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func scanProof(row interface{ Scan(...any) error }, p *Proof) error {
	if err := row.Scan(&p.ID, &p.AgentID, &p.TaskSlug, &p.Status, &p.CreatedAt, &p.ClaimedAt, &p.DiffSubmittedAt, &p.FinishedAt,
		&p.Diff, &p.AgentLogTail, &p.AgentDurationMS, &p.AgentExitCode, &p.SandboxResult, &p.FailureReason); err != nil {
		return err
	}
	p.CreatedAt = p.CreatedAt.UTC()
	p.ClaimedAt, p.DiffSubmittedAt, p.FinishedAt = utcp(p.ClaimedAt), utcp(p.DiffSubmittedAt), utcp(p.FinishedAt)
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

// Tasks lists the catalog without tarballs.
func (s *Service) Tasks(ctx context.Context) ([]Task, error) {
	out := []Task{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT slug, title, language, agent_timeout_s, sandbox_timeout_s, visible_tests, hidden_tests, task_md, repo_sha256 FROM proof_tasks ORDER BY slug`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t Task
			if err := rows.Scan(&t.Slug, &t.Title, &t.Language, &t.AgentTimeoutS, &t.SandboxTimeoutS, &t.VisibleTests, &t.HiddenTests, &t.TaskMD, &t.RepoSHA256); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

// Create queues a proof for the caller's agent. The agent must be online
// (presence within 2 minutes), have no proof in flight and be under the
// daily limit; the partial unique index is the last word on "in flight".
func (s *Service) Create(ctx context.Context, userID, slug string) (Proof, error) {
	var p Proof
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		agentID, err := s.agentOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM proof_tasks WHERE slug = $1)`, slug).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return httpx.NotFound()
		}
		var online bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM agent_presence WHERE agent_id = $1 AND last_seen_at > now() - interval '2 minutes')`, agentID).Scan(&online); err != nil {
			return err
		}
		if !online {
			return httpx.New(http.StatusConflict, "agent_offline", "The connector is not online; run `arena connect` first")
		}
		var today int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM proofs WHERE agent_id = $1 AND created_at > now() - interval '24 hours'`, agentID).Scan(&today); err != nil {
			return err
		}
		if today >= dailyLimit {
			return httpx.New(http.StatusTooManyRequests, "daily_limit", "At most 10 proofs per day per agent")
		}
		if err := scanProof(tx.QueryRow(ctx, `INSERT INTO proofs (id, agent_id, task_slug) VALUES ($1, $2, $3) RETURNING `+proofCols,
			idgen.New("proof"), agentID, slug), &p); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "proof.created", AggregateKind: "proof", AggregateID: p.ID, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "proofs_one_open_idx") {
		return Proof{}, httpx.New(http.StatusConflict, "proof_in_progress", "A proof is already in progress")
	}
	return p, err
}

func (s *Service) List(ctx context.Context, userID string) ([]Proof, error) {
	out := []Proof{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		agentID, err := s.agentOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+proofCols+` FROM proofs WHERE agent_id = $1 ORDER BY created_at DESC LIMIT 50`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p Proof
			if err := scanProof(rows, &p); err != nil {
				return err
			}
			p.Diff, p.AgentLogTail = "", "" // list view stays light
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) Get(ctx context.Context, userID, id string) (Proof, error) {
	var p Proof
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanProof(tx.QueryRow(ctx, `SELECT `+proofCols+` FROM proofs p WHERE p.id = $1
			AND p.agent_id = (SELECT id FROM agents WHERE owner_user_id = $2)`, id, userID), &p)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Proof{}, httpx.NotFound()
	}
	return p, err
}

// Retry re-queues a proof that ended in infra_error or expired, clearing
// everything the previous run produced.
func (s *Service) Retry(ctx context.Context, userID, id string) (Proof, error) {
	var p Proof
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM proofs WHERE id = $1 AND agent_id = (SELECT id FROM agents WHERE owner_user_id = $2) FOR UPDATE`, id, userID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		if status != StatusInfraError && status != StatusExpired {
			return httpx.StateConflict("Only proofs that ended in infra_error or expired can be retried")
		}
		return scanProof(tx.QueryRow(ctx, `UPDATE proofs SET status = 'queued', created_at = now(), claimed_at = NULL, diff_submitted_at = NULL,
			finished_at = NULL, diff = '', agent_log_tail = '', agent_duration_ms = NULL, agent_exit_code = NULL, sandbox_result = NULL, failure_reason = ''
			WHERE id = $1 RETURNING `+proofCols, id), &p)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "proofs_one_open_idx") {
		return Proof{}, httpx.New(http.StatusConflict, "proof_in_progress", "A proof is already in progress")
	}
	return p, err
}

// ProofFacts implements agents.ProofFactsSource.
func (s *Service) ProofFacts(ctx context.Context, agentID string) (agents.ProofFacts, error) {
	var f agents.ProofFacts
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM proofs WHERE agent_id = $1 AND status = 'passed'),
			EXISTS (SELECT 1 FROM proofs WHERE agent_id = $1 AND status = ANY($2)),
			coalesce((SELECT status FROM proofs WHERE agent_id = $1 AND finished_at IS NOT NULL ORDER BY finished_at DESC LIMIT 1), '')`,
			agentID, openStatuses).Scan(&f.HasPassed, &f.HasOpen, &f.LastFinishedStatus)
	})
	return f, err
}

type ResultInput struct {
	Diff       string `json:"diff"`
	LogTail    string `json:"log_tail"`
	DurationMS int    `json:"duration_ms"`
	ExitCode   int    `json:"exit_code"`
}

type RunProofPayload struct {
	ProofID string `json:"proof_id"`
}

func (s *Service) task(ctx context.Context, tx pgx.Tx, slug string) (*Task, error) {
	var t Task
	err := tx.QueryRow(ctx, `SELECT slug, title, language, image, run_cmd, agent_timeout_s, sandbox_timeout_s, visible_tests, hidden_tests, task_md, repo_sha256 FROM proof_tasks WHERE slug = $1`, slug).
		Scan(&t.Slug, &t.Title, &t.Language, &t.Image, &t.RunCmd, &t.AgentTimeoutS, &t.SandboxTimeoutS, &t.VisibleTests, &t.HiddenTests, &t.TaskMD, &t.RepoSHA256)
	return &t, err
}

// Claim hands the agent's oldest queued proof to the connector. SKIP LOCKED
// makes two connectors on one key race safely: one wins, the other sees nil.
func (s *Service) Claim(ctx context.Context, agentID string) (*Proof, *Task, error) {
	var p Proof
	var t *Task
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		err := scanProof(tx.QueryRow(ctx, `UPDATE proofs SET status = 'claimed', claimed_at = now()
			WHERE id = (SELECT id FROM proofs WHERE agent_id = $1 AND status = 'queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
			RETURNING `+proofCols, agentID), &p)
		if err != nil {
			return err
		}
		t, err = s.task(ctx, tx, p.TaskSlug)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &p, t, nil
}

func (s *Service) RepoTar(ctx context.Context, agentID, proofID string) ([]byte, error) {
	var tar []byte
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT t.repo_tar FROM proofs p JOIN proof_tasks t ON t.slug = p.task_slug
			WHERE p.id = $1 AND p.agent_id = $2 AND p.status IN ('claimed', 'running_agent')`, proofID, agentID).Scan(&tar)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.NotFound()
	}
	return tar, err
}

func (s *Service) Started(ctx context.Context, agentID, proofID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE proofs SET status = 'running_agent' WHERE id = $1 AND agent_id = $2 AND status = 'claimed'`, proofID, agentID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.StateConflict("Proof is not in claimed state")
		}
		return nil
	})
}

// SubmitResult stores the agent's diff and enqueues the sandbox run in the
// same transaction, so a stored diff is always followed by a run.
func (s *Service) SubmitResult(ctx context.Context, agentID, proofID string, in ResultInput) error {
	if len(in.Diff) > maxDiffBytes {
		return httpx.New(http.StatusRequestEntityTooLarge, "diff_too_large", "Diff exceeds 256 KiB")
	}
	logTail := sanitize.CleanLog(in.LogTail, maxLogTailBytes)
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE proofs SET status = 'diff_submitted', diff_submitted_at = now(), diff = $3, agent_log_tail = $4,
			agent_duration_ms = $5, agent_exit_code = $6
			WHERE id = $1 AND agent_id = $2 AND status IN ('claimed', 'running_agent')`, proofID, agentID, in.Diff, logTail, in.DurationMS, in.ExitCode)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.StateConflict("Proof already has a result or is not running")
		}
		_, err = jobs.Enqueue(ctx, tx, "run_proof", RunProofPayload{ProofID: proofID}, "run_proof:"+proofID)
		return err
	})
}
