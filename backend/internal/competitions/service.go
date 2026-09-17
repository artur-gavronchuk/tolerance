package competitions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

const cols = `c.id, c.slug, c.title, c.summary, c.brief, c.category, c.difficulty, c.status, c.points, c.deadline,
	c.match_duration_seconds, c.criteria, c.created_by, c.created_at, c.published_at, c.closed_at, c.version,
	(SELECT count(DISTINCT agent_id) FROM submissions s WHERE s.competition_id = c.id)::int,
	(SELECT count(*) FROM submissions s WHERE s.competition_id = c.id AND s.score_status = 'scored')::int`

func scan(row interface{ Scan(...any) error }, c *Competition) error {
	var criteria []byte
	if err := row.Scan(&c.ID, &c.Slug, &c.Title, &c.Summary, &c.Brief, &c.Category, &c.Difficulty, &c.Status, &c.Points, &c.Deadline,
		&c.MatchDurationSeconds, &criteria, &c.CreatedBy, &c.CreatedAt, &c.PublishedAt, &c.ClosedAt, &c.Version, &c.Participants, &c.ScoredCount); err != nil {
		return err
	}
	return json.Unmarshal(criteria, &c.Criteria)
}

func requireAdmin(actor identity.Actor) error {
	if actor.Role != "admin" {
		return httpx.Forbidden("Admin role required")
	}
	return nil
}

func (s *Service) Create(ctx context.Context, actor identity.Actor, in Input) (Competition, error) {
	if err := requireAdmin(actor); err != nil {
		return Competition{}, err
	}
	if err := Validate(in, time.Now()); err != nil {
		return Competition{}, err
	}
	criteria, _ := json.Marshal(in.Criteria)
	id := idgen.New("comp")
	var c Competition
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO competitions (id, slug, title, summary, brief, category, difficulty, status, points, deadline, match_duration_seconds, criteria, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'draft',$8,$9,$10,$11,$12)`,
			id, in.Slug, in.Title, in.Summary, in.Brief, in.Category, in.Difficulty, in.Points, in.Deadline.UTC(), in.MatchDurationSeconds, criteria, actor.UserID); err != nil {
			return err
		}
		if err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1`, id), &c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actor.ID, ActorKind: actor.Kind, Action: "competition.created",
			AggregateKind: "competition", AggregateID: id, AfterVersion: &c.Version, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Competition{}, httpx.New(http.StatusConflict, "slug_taken", "This slug is already used")
	}
	return c, err
}

func (s *Service) Update(ctx context.Context, actor identity.Actor, id string, in Input) (Competition, error) {
	if err := requireAdmin(actor); err != nil {
		return Competition{}, err
	}
	if err := Validate(in, time.Now()); err != nil {
		return Competition{}, err
	}
	criteria, _ := json.Marshal(in.Criteria)
	var c Competition
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1 FOR UPDATE OF c`, id), &c); err != nil {
			return err
		}
		if c.Status != StatusDraft {
			return httpx.StateConflict("Only draft competitions can be edited")
		}
		before := c.Version
		if _, err := tx.Exec(ctx, `UPDATE competitions SET slug=$2, title=$3, summary=$4, brief=$5, category=$6, difficulty=$7, points=$8, deadline=$9,
			match_duration_seconds=$10, criteria=$11, version = version + 1 WHERE id = $1`,
			id, in.Slug, in.Title, in.Summary, in.Brief, in.Category, in.Difficulty, in.Points, in.Deadline.UTC(), in.MatchDurationSeconds, criteria); err != nil {
			return err
		}
		if err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1`, id), &c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actor.ID, ActorKind: actor.Kind, Action: "competition.updated",
			AggregateKind: "competition", AggregateID: id, BeforeVersion: &before, AfterVersion: &c.Version, RequestID: httpx.RequestID(ctx)})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Competition{}, httpx.NotFound()
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Competition{}, httpx.New(http.StatusConflict, "slug_taken", "This slug is already used")
	}
	return c, err
}

func (s *Service) Delete(ctx context.Context, actor identity.Actor, id string) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM competitions WHERE id = $1 FOR UPDATE`, id).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		if status != StatusDraft {
			return httpx.StateConflict("Only draft competitions can be deleted")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM competitions WHERE id = $1`, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actor.ID, ActorKind: actor.Kind, Action: "competition.deleted",
			AggregateKind: "competition", AggregateID: id, RequestID: httpx.RequestID(ctx)})
	})
}

// transition moves a competition between statuses with optimistic
// concurrency; a stale expected_version yields 409 state_conflict.
func (s *Service) transition(ctx context.Context, actor identity.Actor, id string, expected int, from, to, set, action, reason string) (Competition, error) {
	var c Competition
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1 FOR UPDATE OF c`, id), &c); err != nil {
			return err
		}
		if c.Status != from {
			return httpx.StateConflict("Competition is " + c.Status)
		}
		before := c.Version
		tag, err := tx.Exec(ctx, `UPDATE competitions SET status = $2, `+set+`, version = version + 1 WHERE id = $1 AND version = $3`, id, to, expected)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.StateConflict("Competition changed; reload and retry")
		}
		if err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1`, id), &c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actor.ID, ActorKind: actor.Kind, Action: action, Reason: reason,
			AggregateKind: "competition", AggregateID: id, BeforeVersion: &before, AfterVersion: &c.Version, RequestID: httpx.RequestID(ctx)})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Competition{}, httpx.NotFound()
	}
	return c, err
}

func (s *Service) Publish(ctx context.Context, actor identity.Actor, id string, expected int) (Competition, error) {
	if err := requireAdmin(actor); err != nil {
		return Competition{}, err
	}
	c, err := s.GetByID(ctx, id)
	if err != nil {
		return Competition{}, err
	}
	if !c.Deadline.After(time.Now().Add(time.Hour)) {
		return Competition{}, httpx.StateConflict("Deadline must be at least one hour away to publish")
	}
	return s.transition(ctx, actor, id, expected, StatusDraft, StatusActive, "published_at = now()", "competition.published", "")
}

func (s *Service) Close(ctx context.Context, actor identity.Actor, id string, expected int, reason string) (Competition, error) {
	if err := requireAdmin(actor); err != nil {
		return Competition{}, err
	}
	if !httpx.ValidText(reason, 1, 2000) {
		return Competition{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "reason is required", "reason", "required")
	}
	return s.transition(ctx, actor, id, expected, StatusActive, StatusClosed, "closed_at = now()", "competition.closed", reason)
}

func (s *Service) GetByID(ctx context.Context, id string) (Competition, error) {
	var c Competition
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.id = $1`, id), &c)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Competition{}, httpx.NotFound()
	}
	return c, err
}

func (s *Service) GetPublicBySlug(ctx context.Context, slug string) (Competition, error) {
	var c Competition
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM competitions c WHERE c.slug = $1 AND c.status <> 'draft'`, slug), &c)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Competition{}, httpx.NotFound()
	}
	return c, err
}

func (s *Service) list(ctx context.Context, where string, args ...any) ([]Competition, error) {
	out := []Competition{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+cols+` FROM competitions c `+where, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c Competition
			if err := scan(rows, &c); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// ListPublic: status "active" | "past" | "" (both). Active by deadline asc,
// past by deadline desc.
func (s *Service) ListPublic(ctx context.Context, status string) ([]Competition, error) {
	switch status {
	case "active":
		return s.list(ctx, `WHERE c.status = 'active' ORDER BY c.deadline ASC`)
	case "past":
		return s.list(ctx, `WHERE c.status = 'closed' ORDER BY c.deadline DESC`)
	case "":
		return s.list(ctx, `WHERE c.status <> 'draft' ORDER BY (c.status = 'active') DESC, CASE WHEN c.status = 'active' THEN c.deadline END ASC, c.deadline DESC`)
	}
	return nil, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "status must be active or past", "status", "invalid")
}

func (s *Service) ListAdmin(ctx context.Context) ([]Competition, error) {
	return s.list(ctx, `ORDER BY c.created_at DESC`)
}

// CloseExpired is the scheduler's step: every active competition whose
// deadline has passed becomes closed, with a system audit event each.
func (s *Service) CloseExpired(ctx context.Context) (int, error) {
	n := 0
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE competitions SET status = 'closed', closed_at = now(), version = version + 1
			WHERE status = 'active' AND deadline <= now() RETURNING id, version`)
		if err != nil {
			return err
		}
		type closedRow struct {
			id      string
			version int
		}
		var ids []closedRow
		for rows.Next() {
			var c closedRow
			if err := rows.Scan(&c.id, &c.version); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, c := range ids {
			before := c.version - 1
			if err := audit.Record(ctx, tx, audit.Event{ActorID: identity.System.ID, ActorKind: identity.KindSystem, Action: "competition.closed",
				Reason: "deadline", AggregateKind: "competition", AggregateID: c.id, BeforeVersion: &before, AfterVersion: &c.version}); err != nil {
				return err
			}
		}
		n = len(ids)
		return nil
	})
	return n, err
}
