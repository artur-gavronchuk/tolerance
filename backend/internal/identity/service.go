package identity

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	pool        *db.Pool
	adminEmails map[string]bool
}

func NewService(pool *db.Pool, adminEmails []string) *Service {
	m := map[string]bool{}
	for _, e := range adminEmails {
		if e = NormalizeEmail(e); e != "" {
			m[e] = true
		}
	}
	return &Service{pool: pool, adminEmails: m}
}

// NormalizeEmail trims and lower-cases; the database stores this form only.
func NormalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

const userColumns = `id, email, role, created_at`

func scanUser(row interface{ Scan(...any) error }, u *User) error {
	if err := row.Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt); err != nil {
		return err
	}
	u.CreatedAt = u.CreatedAt.UTC()
	return nil
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

const minPasswordLen = 10

func validateCredentials(email, password string) error {
	if _, err := mail.ParseAddress(email); err != nil || len(email) > 254 {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "email must be a valid address", "email", "invalid")
	}
	if len(password) < minPasswordLen || len(password) > 1024 {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "password must be at least 10 characters", "password", "too_short")
	}
	return nil
}

func (s *Service) roleFor(email string) string {
	if s.adminEmails[email] {
		return "admin"
	}
	return "user"
}

// Signup creates the user and a first session. The token goes into the
// cookie; only its hash is stored.
func (s *Service) Signup(ctx context.Context, email, password string) (User, string, error) {
	email = NormalizeEmail(email)
	if err := validateCredentials(email, password); err != nil {
		return User{}, "", err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, "", err
	}
	token, sid, err := newSessionToken()
	if err != nil {
		return User{}, "", err
	}
	var u User
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanUser(tx.QueryRow(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING `+userColumns,
			idgen.New("user"), email, hash, s.roleFor(email)), &u); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, now() + $3::interval)`, sid, u.ID, sessionTTL.String()); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: u.ID, ActorKind: KindUser, Action: "user.signed_up",
			AggregateKind: "user", AggregateID: u.ID, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return User{}, "", httpx.New(http.StatusConflict, "email_taken", "An account with this email already exists")
	}
	if err != nil {
		return User{}, "", err
	}
	return u, token, nil
}

var errInvalidCredentials = httpx.New(http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect")

func (s *Service) Login(ctx context.Context, email, password string) (User, string, error) {
	email = NormalizeEmail(email)
	var u User
	var hash string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT `+userColumns+`, password_hash FROM users WHERE email = $1`, email).
			Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt, &hash)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Burn the same time as a real verification so timing does not reveal
		// whether the email exists.
		VerifyPassword("$argon2id$v=19$m=65536,t=1,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", password)
		return User{}, "", errInvalidCredentials
	}
	if err != nil {
		return User{}, "", err
	}
	if !VerifyPassword(hash, password) {
		return User{}, "", errInvalidCredentials
	}
	u.CreatedAt = u.CreatedAt.UTC()
	token, sid, err := newSessionToken()
	if err != nil {
		return User{}, "", err
	}
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1`, u.ID, s.roleFor(email)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, now() + $3::interval)`, sid, u.ID, sessionTTL.String())
		return err
	})
	if err != nil {
		return User{}, "", err
	}
	u.Role = s.roleFor(email)
	return u, token, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID(token))
		return err
	})
}

// UserBySession resolves a cookie token; expired sessions are deleted on
// sight. last_seen_at is bumped at most once an hour.
func (s *Service) UserBySession(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrNoSession
	}
	var u User
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanUser(tx.QueryRow(ctx, `
			UPDATE sessions s SET last_seen_at = CASE WHEN s.last_seen_at < now() - interval '1 hour' THEN now() ELSE s.last_seen_at END
			FROM users u WHERE s.id = $1 AND s.user_id = u.id AND s.expires_at > now()
			RETURNING u.id, u.email, u.role, u.created_at`, sessionID(token)), &u)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNoSession
	}
	return u, err
}
