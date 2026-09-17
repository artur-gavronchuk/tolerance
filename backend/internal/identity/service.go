package identity

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

type User struct {
	ID          string    `json:"id"`
	Handle      string    `json:"handle"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email,omitempty"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

type Service struct {
	pool        *db.Pool
	adminEmails map[string]bool
}

func NewService(pool *db.Pool, adminEmails []string) *Service {
	m := map[string]bool{}
	for _, e := range adminEmails {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			m[e] = true
		}
	}
	return &Service{pool: pool, adminEmails: m}
}

const userColumns = `id, handle, display_name, coalesce(email, ''), role, created_at`

func scanUser(row interface{ Scan(...any) error }, u *User) error {
	return row.Scan(&u.ID, &u.Handle, &u.DisplayName, &u.Email, &u.Role, &u.CreatedAt)
}

// ResolveUser finds or creates the user for a verified token. Lookup order:
// (issuer, subject); then a seed user (oidc_issuer = 'seed') with the same
// email, which is adopted by rewriting its issuer/subject; otherwise a new
// row with a unique handle. Admin role follows ARENA_ADMIN_EMAILS on every
// login so the list can change without touching the database.
func (s *Service) ResolveUser(ctx context.Context, c auth.Claims) (User, error) {
	var u User
	role := "user"
	if s.adminEmails[strings.ToLower(c.Email)] {
		role = "admin"
	}
	displayName := strings.TrimSpace(c.Name)
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		err := scanUser(tx.QueryRow(ctx, `
			UPDATE users SET display_name = coalesce(nullif($3, ''), display_name),
			                 email = coalesce(nullif($4, ''), email), role = $5
			WHERE oidc_issuer = $1 AND oidc_subject = $2
			RETURNING `+userColumns, c.Issuer, c.Subject, displayName, c.Email, role), &u)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if c.Email != "" {
			err = scanUser(tx.QueryRow(ctx, `
				UPDATE users SET oidc_issuer = $1, oidc_subject = $2, role = $4,
				                 display_name = coalesce(nullif($3, ''), display_name)
				WHERE oidc_issuer = 'seed' AND lower(email) = lower($5)
				RETURNING `+userColumns, c.Issuer, c.Subject, displayName, role, c.Email), &u)
			if err == nil {
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		base := DeriveHandle(c)
		if displayName == "" {
			displayName = base
		}
		for i := 1; i <= 50; i++ {
			handle := base
			if i > 1 {
				suffix := fmt.Sprintf("-%d", i)
				handle = base + suffix
				if len(handle) > 32 {
					handle = base[:32-len(suffix)] + suffix
				}
			}
			err = scanUser(tx.QueryRow(ctx, `
				INSERT INTO users (id, oidc_issuer, oidc_subject, email, handle, display_name, role)
				VALUES ($1, $2, $3, nullif($4, ''), $5, $6, $7)
				ON CONFLICT (handle) DO NOTHING
				RETURNING `+userColumns,
				idgen.New("user"), c.Issuer, c.Subject, c.Email, handle, displayName, role), &u)
			if err == nil {
				return audit.Record(ctx, tx, audit.Event{ActorID: u.ID, ActorKind: KindUser, Action: "user.created",
					AggregateKind: "user", AggregateID: u.ID, RequestID: httpx.RequestID(ctx)})
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		return errors.New("identity: could not find a free handle")
	})
	if err != nil {
		return User{}, fmt.Errorf("resolve user: %w", err)
	}
	return u, nil
}

func (s *Service) Get(ctx context.Context, id string) (User, error) {
	var u User
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id), &u)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, httpx.NotFound()
	}
	return u, err
}

type UpdateInput struct {
	DisplayName *string `json:"display_name"`
	Handle      *string `json:"handle"`
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (User, error) {
	if in.DisplayName != nil && !httpx.ValidText(*in.DisplayName, 1, 80) {
		return User{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "display_name must be 1-80 characters", "display_name", "invalid")
	}
	if in.Handle != nil && !ValidHandle(*in.Handle) {
		return User{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "handle must match ^[a-z0-9][a-z0-9-]{1,31}$", "handle", "invalid")
	}
	var u User
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		err := scanUser(tx.QueryRow(ctx, `
			UPDATE users SET display_name = coalesce($2, display_name), handle = coalesce($3, handle)
			WHERE id = $1 RETURNING `+userColumns, id, in.DisplayName, in.Handle), &u)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: id, ActorKind: KindUser, Action: "user.updated",
			AggregateKind: "user", AggregateID: id, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return User{}, httpx.New(http.StatusConflict, "handle_taken", "This handle is already taken")
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, httpx.NotFound()
	}
	return u, err
}
