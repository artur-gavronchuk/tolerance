package agents

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/standings"
)

const maxActiveKeys = 5

type Service struct {
	pool      *db.Pool
	standings *standings.Service
}

func NewService(pool *db.Pool, st *standings.Service) *Service {
	return &Service{pool: pool, standings: st}
}

const agentCols = `id, owner_user_id, name, model, bio, created_at, version`

func scanAgent(row interface{ Scan(...any) error }, a *Agent) error {
	return row.Scan(&a.ID, &a.OwnerUserID, &a.Name, &a.Model, &a.Bio, &a.CreatedAt, &a.Version)
}

func validateCreate(in CreateInput) error {
	if !nameRe.MatchString(in.Name) {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "name must match ^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$", "name", "invalid")
	}
	if !httpx.ValidText(in.Model, 1, 80) {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "model must be 1-80 characters", "model", "invalid")
	}
	if len(in.Bio) > 500 {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "bio must be at most 500 characters", "bio", "too_long")
	}
	return nil
}

func uniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, constraint)
}

func (s *Service) Create(ctx context.Context, ownerID string, in CreateInput) (Private, error) {
	if err := validateCreate(in); err != nil {
		return Private{}, err
	}
	a := Agent{ID: idgen.New("agent"), OwnerUserID: ownerID, Name: in.Name, Model: strings.TrimSpace(in.Model), Bio: in.Bio, CreatedAt: time.Now().UTC(), Version: 1}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO agents (id, owner_user_id, name, model, bio, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
			a.ID, a.OwnerUserID, a.Name, a.Model, a.Bio, a.CreatedAt); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: ownerID, ActorKind: identity.KindUser, Action: "agent.created",
			AggregateKind: "agent", AggregateID: a.ID, RequestID: httpx.RequestID(ctx)})
	})
	switch {
	case uniqueViolation(err, "agents_owner_user_id_key"):
		return Private{}, httpx.New(http.StatusConflict, "agent_exists", "You already have an agent")
	case uniqueViolation(err, "agents_name_ci_idx"):
		return Private{}, httpx.New(http.StatusConflict, "name_taken", "This agent name is already taken")
	case err != nil:
		return Private{}, err
	}
	return s.private(ctx, a)
}

func (s *Service) Patch(ctx context.Context, ownerID string, in PatchInput) (Private, error) {
	if in.Model != nil && !httpx.ValidText(*in.Model, 1, 80) {
		return Private{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "model must be 1-80 characters", "model", "invalid")
	}
	if in.Bio != nil && len(*in.Bio) > 500 {
		return Private{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "bio must be at most 500 characters", "bio", "too_long")
	}
	var a Agent
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanAgent(tx.QueryRow(ctx, `UPDATE agents SET model = coalesce($2, model), bio = coalesce($3, bio), version = version + 1
			WHERE owner_user_id = $1 RETURNING `+agentCols, ownerID, in.Model, in.Bio), &a); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: ownerID, ActorKind: identity.KindUser, Action: "agent.updated",
			AggregateKind: "agent", AggregateID: a.ID, RequestID: httpx.RequestID(ctx)})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Private{}, httpx.NotFound()
	}
	if err != nil {
		return Private{}, err
	}
	return s.private(ctx, a)
}

func (s *Service) private(ctx context.Context, a Agent) (Private, error) {
	p := Private{ID: a.ID, Name: a.Name, Model: a.Model, Bio: a.Bio, CreatedAt: a.CreatedAt, APIKeys: []KeyView{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM arena_queue WHERE agent_id = $1)`, a.ID).Scan(&p.InArenaQueue); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id, prefix, name, created_at, last_used_at FROM api_keys WHERE agent_id = $1 AND revoked_at IS NULL ORDER BY created_at`, a.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k KeyView
			if err := rows.Scan(&k.ID, &k.Prefix, &k.Name, &k.CreatedAt, &k.LastUsedAt); err != nil {
				return err
			}
			p.APIKeys = append(p.APIKeys, k)
		}
		return rows.Err()
	})
	return p, err
}

func (s *Service) byOwner(ctx context.Context, ownerID string) (Agent, error) {
	var a Agent
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanAgent(tx.QueryRow(ctx, `SELECT `+agentCols+` FROM agents WHERE owner_user_id = $1`, ownerID), &a)
	})
	return a, err
}

// PrivateForUser returns nil, nil when the user has no agent yet.
func (s *Service) PrivateForUser(ctx context.Context, userID string) (*Private, error) {
	a, err := s.byOwner(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p, err := s.private(ctx, a)
	return &p, err
}

// MeAgent adapts PrivateForUser for identity.RegisterRoutes: a missing
// agent must serialise as JSON null, which a typed nil pointer boxed in an
// any would not (it would encode as {"...": nil-ish struct}); returning an
// untyped nil interface value does encode as null.
func (s *Service) MeAgent(ctx context.Context, userID string) (any, error) {
	p, err := s.PrivateForUser(ctx, userID)
	if err != nil || p == nil {
		return nil, err
	}
	return p, nil
}

func (s *Service) ByName(ctx context.Context, name string) (Agent, error) {
	var a Agent
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanAgent(tx.QueryRow(ctx, `SELECT `+agentCols+` FROM agents WHERE lower(name) = lower($1)`, name), &a)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, httpx.NotFound()
	}
	return a, err
}

func (s *Service) ByID(ctx context.Context, id string) (Agent, error) {
	var a Agent
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanAgent(tx.QueryRow(ctx, `SELECT `+agentCols+` FROM agents WHERE id = $1`, id), &a)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, httpx.NotFound()
	}
	return a, err
}

func (s *Service) ProfileByName(ctx context.Context, name string) (Profile, standings.Standing, error) {
	a, err := s.ByName(ctx, name)
	if err != nil {
		return Profile{}, standings.Standing{}, err
	}
	st, err := s.standings.ForAgent(ctx, a.ID)
	if err != nil {
		return Profile{}, standings.Standing{}, err
	}
	badges, err := s.standings.BadgesForAgent(ctx, a.ID)
	if err != nil {
		return Profile{}, standings.Standing{}, err
	}
	return Profile{Agent: a.Name, Author: st.Author, Model: a.Model, Bio: a.Bio, Joined: a.CreatedAt, Badges: badges}, st, nil
}

func (s *Service) CreateKey(ctx context.Context, ownerID, name string) (KeyView, string, error) {
	if len(name) > 40 {
		return KeyView{}, "", httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "name must be at most 40 characters", "name", "too_long")
	}
	a, err := s.byOwner(ctx, ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return KeyView{}, "", httpx.NotFound()
	}
	if err != nil {
		return KeyView{}, "", err
	}
	key, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		return KeyView{}, "", err
	}
	kv := KeyView{ID: idgen.New("key"), Prefix: prefix, Name: name, CreatedAt: time.Now().UTC()}
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Lock the agent row (not an aggregate query, which Postgres refuses
		// to combine with FOR UPDATE) so two concurrent key creations for
		// the same agent serialize instead of both passing the count check.
		var lockedID string
		if err := tx.QueryRow(ctx, `SELECT id FROM agents WHERE id = $1 FOR UPDATE`, a.ID).Scan(&lockedID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM api_keys WHERE agent_id = $1 AND revoked_at IS NULL`, a.ID).Scan(&n); err != nil {
			return err
		}
		if n >= maxActiveKeys {
			return httpx.New(http.StatusConflict, "too_many_keys", "At most 5 active API keys per agent")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO api_keys (id, agent_id, prefix, key_hash, name, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
			kv.ID, a.ID, prefix, hash, name, kv.CreatedAt); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: ownerID, ActorKind: identity.KindUser, Action: "api_key.created",
			AggregateKind: "agent", AggregateID: a.ID, Payload: map[string]any{"key_id": kv.ID}, RequestID: httpx.RequestID(ctx)})
	})
	if err != nil {
		return KeyView{}, "", err
	}
	return kv, key, nil
}

func (s *Service) RevokeKey(ctx context.Context, ownerID, keyID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE api_keys k SET revoked_at = now() FROM agents a
			WHERE k.id = $1 AND k.agent_id = a.id AND a.owner_user_id = $2 AND k.revoked_at IS NULL`, keyID, ownerID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NotFound()
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: ownerID, ActorKind: identity.KindUser, Action: "api_key.revoked",
			AggregateKind: "api_key", AggregateID: keyID, RequestID: httpx.RequestID(ctx)})
	})
}

// AgentIDByKeyHash implements identity.AgentLookup. last_used_at is bumped
// at most once a minute to keep the hot path to a single indexed read.
func (s *Service) AgentIDByKeyHash(ctx context.Context, hash string) (string, error) {
	var id string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `UPDATE api_keys SET last_used_at = CASE WHEN last_used_at IS NULL OR last_used_at < now() - interval '1 minute' THEN now() ELSE last_used_at END
			WHERE key_hash = $1 AND revoked_at IS NULL RETURNING agent_id`, hash).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", identity.ErrNoAgent
	}
	return id, err
}
