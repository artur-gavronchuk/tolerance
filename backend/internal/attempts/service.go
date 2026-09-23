package attempts

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

type Service struct {
	pool   *db.Pool
	events *limiter
}

func NewService(pool *db.Pool) *Service {
	return &Service{pool: pool, events: newLimiter(4, 4, time.Now)} // 4 event batches per second per attempt
}

func conflict(code, message string) *httpx.Problem {
	return httpx.New(http.StatusConflict, code, message)
}

func requireAgent(actor identity.Actor) error {
	if actor.Kind != identity.KindAgent || actor.AgentID == "" {
		return httpx.Forbidden("An agent API key is required")
	}
	return nil
}

func (c AgentConfig) validate() error {
	for path, v := range map[string]string{
		"agent_config.adapter": c.Adapter, "agent_config.adapter_model": c.AdapterModel,
		"agent_config.connector_version": c.ConnectorVersion, "agent_config.os": c.OS,
	} {
		if len(v) > maxFieldLen {
			return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "agent_config fields are at most 80 characters", path, "too_long")
		}
	}
	return nil
}

const attemptCols = `a.id, a.competition_id, c.slug, a.agent_id, a.kind, a.attempt_no, a.status, a.agent_snapshot,
	a.match_id, a.started_at, a.finished_at, a.voided_at IS NOT NULL`

func scanAttempt(row interface{ Scan(...any) error }, a *Attempt) error {
	if err := row.Scan(&a.ID, &a.CompetitionID, &a.CompetitionSlug, &a.AgentID, &a.Kind, &a.No, &a.Status,
		&a.AgentSnapshot, &a.MatchID, &a.StartedAt, &a.FinishedAt, &a.Voided); err != nil {
		return err
	}
	a.StartedAt = a.StartedAt.UTC()
	if a.FinishedAt != nil {
		t := a.FinishedAt.UTC()
		a.FinishedAt = &t
	}
	return nil
}

func uniqueConstraint(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName
	}
	return ""
}

// Start begins an attempt, or returns the agent's attempt that is still
// running (resumed = true). An official attempt can be started once; it
// is only available again after an admin voids it.
func (s *Service) Start(ctx context.Context, actor identity.Actor, slug, kind string, cfg AgentConfig) (a Attempt, resumed bool, err error) {
	if err := requireAgent(actor); err != nil {
		return Attempt{}, false, err
	}
	if kind != KindOfficial && kind != KindPractice {
		return Attempt{}, false, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "kind must be official or practice", "kind", "invalid")
	}
	if err := cfg.validate(); err != nil {
		return Attempt{}, false, err
	}
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var compID, status string
		var deadline time.Time
		if err := tx.QueryRow(ctx, `SELECT id, status, deadline FROM competitions WHERE slug = $1 FOR SHARE`, slug).Scan(&compID, &status, &deadline); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		if status != "active" {
			return httpx.NotFound()
		}
		if !time.Now().Before(deadline) {
			return conflict("deadline_passed", "The deadline for this competition has passed")
		}

		// Serialise starts per agent so numbering and the resume rule are race-free.
		var name, model, bio string
		if err := tx.QueryRow(ctx, `SELECT name, model, bio FROM agents WHERE id = $1 FOR UPDATE`, actor.AgentID).Scan(&name, &model, &bio); err != nil {
			return err
		}

		err := scanAttempt(tx.QueryRow(ctx, `SELECT `+attemptCols+` FROM attempts a JOIN competitions c ON c.id = a.competition_id
			WHERE a.competition_id = $1 AND a.agent_id = $2 AND a.status = 'running' AND a.voided_at IS NULL`, compID, actor.AgentID), &a)
		if err == nil {
			resumed = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		if kind == KindOfficial {
			var used bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attempts WHERE competition_id = $1 AND agent_id = $2
				AND kind = 'official' AND voided_at IS NULL)`, compID, actor.AgentID).Scan(&used); err != nil {
				return err
			}
			if used {
				return conflict("official_attempt_used", "The official attempt for this competition was already used; you can start a practice attempt")
			}
		}

		var no int
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(attempt_no), 0) + 1 FROM attempts WHERE competition_id = $1 AND agent_id = $2`,
			compID, actor.AgentID).Scan(&no); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(Snapshot{Name: name, Model: model, Bio: bio, Adapter: cfg.Adapter,
			AdapterModel: cfg.AdapterModel, ConnectorVersion: cfg.ConnectorVersion, OS: cfg.OS})
		id := idgen.New("att")
		if _, err := tx.Exec(ctx, `INSERT INTO attempts (id, competition_id, agent_id, kind, attempt_no, agent_snapshot)
			VALUES ($1, $2, $3, $4, $5, $6)`, id, compID, actor.AgentID, kind, no, snapshot); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO attempt_events (attempt_id, kind, payload) VALUES ($1, 'started', $2)`,
			id, mustJSON(map[string]any{"attempt_kind": kind, "attempt_no": no})); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Event{ActorID: actor.ID, ActorKind: actor.Kind, Action: "attempt.started",
			AggregateKind: "attempt", AggregateID: id, Payload: map[string]any{"kind": kind, "no": no}, RequestID: httpx.RequestID(ctx)}); err != nil {
			return err
		}
		return scanAttempt(tx.QueryRow(ctx, `SELECT `+attemptCols+` FROM attempts a JOIN competitions c ON c.id = a.competition_id WHERE a.id = $1`, id), &a)
	})
	if name := uniqueConstraint(err); name != "" {
		if name == "attempts_one_official" {
			return Attempt{}, false, conflict("official_attempt_used", "The official attempt for this competition was already used")
		}
		return Attempt{}, false, httpx.StateConflict("An attempt is already in progress")
	}
	return a, resumed, err
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // only ever called with maps of plain values
	}
	return b
}

// ownRunning loads an attempt of the calling agent that can still take
// events. A missing or foreign attempt is a 404 either way.
func ownRunning(ctx context.Context, tx pgx.Tx, actor identity.Actor, attemptID string) (Attempt, error) {
	var a Attempt
	err := scanAttempt(tx.QueryRow(ctx, `SELECT `+attemptCols+` FROM attempts a JOIN competitions c ON c.id = a.competition_id
		WHERE a.id = $1 AND a.agent_id = $2 FOR UPDATE OF a`, attemptID, actor.AgentID), &a)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attempt{}, httpx.NotFound()
	}
	if err != nil {
		return Attempt{}, err
	}
	if a.Status != StatusRunning || a.Voided {
		return Attempt{}, conflict("attempt_not_running", "This attempt is no longer running")
	}
	return a, nil
}

// AddEvents stores connector events after validating and sanitising them.
// The batch is all-or-nothing; a log event whose text is empty after
// cleaning carries nothing and is dropped.
func (s *Service) AddEvents(ctx context.Context, actor identity.Actor, attemptID string, in []EventInput) error {
	if err := requireAgent(actor); err != nil {
		return err
	}
	if len(in) == 0 || len(in) > maxEventsPerBatch {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "send between 1 and 20 events", "events", "invalid_count")
	}
	type row struct {
		kind  string
		phase *int
		text  string
	}
	rows := make([]row, 0, len(in))
	for i, e := range in {
		path := "events[" + strconv.Itoa(i) + "]"
		if !connectorKinds[e.Kind] {
			return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "This event kind cannot be sent by a connector", path+".kind", "invalid_kind")
		}
		if e.PhaseIndex != nil && (*e.PhaseIndex < 0 || *e.PhaseIndex > maxPhaseIndex) {
			return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "phase_index must be 0-8", path+".phase_index", "out_of_range")
		}
		if e.Kind == "phase" && e.PhaseIndex == nil {
			return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "phase events need a phase_index", path+".phase_index", "required")
		}
		text := CleanText(e.Text)
		if e.Kind == "log" && text == "" {
			continue
		}
		rows = append(rows, row{e.Kind, e.PhaseIndex, text})
	}
	if !s.events.Allow(attemptID) {
		return httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many event batches; send at most 4 per second")
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := ownRunning(ctx, tx, actor, attemptID); err != nil {
			return err
		}
		for _, r := range rows {
			var text any
			if r.text != "" {
				text = r.text
			}
			if _, err := tx.Exec(ctx, `INSERT INTO attempt_events (attempt_id, kind, phase_index, text) VALUES ($1, $2, $3, $4)`,
				attemptID, r.kind, r.phase, text); err != nil {
				return err
			}
		}
		return nil
	})
}

// Abandon ends a running attempt without a submission. It does not free the
// official slot: only an admin void does.
func (s *Service) Abandon(ctx context.Context, actor identity.Actor, attemptID string) error {
	if err := requireAgent(actor); err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM attempts WHERE id = $1 AND agent_id = $2 FOR UPDATE`, attemptID, actor.AgentID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		switch status {
		case StatusAbandoned:
			return nil
		case StatusSubmitted:
			return conflict("attempt_not_running", "This attempt was already submitted")
		}
		if _, err := tx.Exec(ctx, `UPDATE attempts SET status = 'abandoned', finished_at = now() WHERE id = $1`, attemptID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO attempt_events (attempt_id, kind) VALUES ($1, 'abandoned')`, attemptID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actor.ID, ActorKind: actor.Kind, Action: "attempt.abandoned",
			AggregateKind: "attempt", AggregateID: attemptID, RequestID: httpx.RequestID(ctx)})
	})
}

// Void cancels an attempt so the agent may start a new official one. It is
// refused once a submission exists: a scored result is never erased this way.
func (s *Service) Void(ctx context.Context, admin identity.Actor, attemptID, reason string) error {
	if admin.Role != "admin" {
		return httpx.Forbidden("Admin role required")
	}
	if !httpx.ValidText(reason, 10, 500) {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "reason must be 10-500 characters", "reason", "invalid")
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var status string
		var voided bool
		err := tx.QueryRow(ctx, `SELECT status, voided_at IS NOT NULL FROM attempts WHERE id = $1 FOR UPDATE`, attemptID).Scan(&status, &voided)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		if voided {
			return httpx.StateConflict("This attempt is already voided")
		}
		var hasSubmission bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM submissions WHERE attempt_id = $1)`, attemptID).Scan(&hasSubmission); err != nil {
			return err
		}
		if hasSubmission {
			return conflict("has_submission", "This attempt already has a submission and cannot be voided")
		}
		if _, err := tx.Exec(ctx, `UPDATE attempts SET voided_at = now(), void_reason = $2,
			status = CASE WHEN status = 'running' THEN 'abandoned' ELSE status END,
			finished_at = coalesce(finished_at, now()) WHERE id = $1`, attemptID, reason); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: admin.ID, ActorKind: admin.Kind, Action: "attempt.voided", Reason: reason,
			AggregateKind: "attempt", AggregateID: attemptID, RequestID: httpx.RequestID(ctx)})
	})
}

// RecordSystemEvent appends an event the server itself produces (a
// submission, a check starting or finishing, the final result).
func (s *Service) RecordSystemEvent(ctx context.Context, tx pgx.Tx, attemptID, kind string, payload any) error {
	if !serverKinds[kind] {
		return errors.New("attempts: " + kind + " is not a server event kind")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO attempt_events (attempt_id, kind, payload) VALUES ($1, $2, $3)`, attemptID, kind, body)
	return err
}

// MarkSubmitted closes a running attempt when its submission is created.
func (s *Service) MarkSubmitted(ctx context.Context, tx pgx.Tx, attemptID string) error {
	tag, err := tx.Exec(ctx, `UPDATE attempts SET status = 'submitted', finished_at = now()
		WHERE id = $1 AND status = 'running' AND voided_at IS NULL`, attemptID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return conflict("attempt_not_running", "This attempt is no longer running")
	}
	return nil
}

// ListForAgent returns the agent's attempts at a competition, oldest first,
// including voided ones (they keep their numbers).
func (s *Service) ListForAgent(ctx context.Context, agentID, competitionID string) ([]Attempt, error) {
	out := []Attempt{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+attemptCols+` FROM attempts a JOIN competitions c ON c.id = a.competition_id
			WHERE a.agent_id = $1 AND a.competition_id = $2 ORDER BY a.attempt_no`, agentID, competitionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a Attempt
			if err := scanAttempt(rows, &a); err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// OfficialAvailable reports whether the agent can still start an official
// attempt at the competition.
func (s *Service) OfficialAvailable(ctx context.Context, agentID, competitionID string) (bool, error) {
	var used bool
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attempts WHERE competition_id = $1 AND agent_id = $2
			AND kind = 'official' AND voided_at IS NULL)`, competitionID, agentID).Scan(&used)
	})
	return !used, err
}

// Events returns an attempt's events with id > afterID, oldest first.
func (s *Service) Events(ctx context.Context, attemptID string, afterID int64, limit int) ([]Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	out := []Event{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, kind, phase_index, text, payload, at FROM attempt_events
			WHERE attempt_id = $1 AND id > $2 ORDER BY id LIMIT $3`, attemptID, afterID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e Event
			if err := rows.Scan(&e.ID, &e.Kind, &e.PhaseIndex, &e.Text, &e.Payload, &e.At); err != nil {
				return err
			}
			e.At = e.At.UTC()
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}
