package identity

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

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

func (s *Service) roleFor(email string) string {
	if s.adminEmails[email] {
		return "admin"
	}
	return "user"
}

// Identity is who a sign-in provider says the person is.
type Identity struct {
	Provider      string // "github" | "google" | "dev"
	Subject       string // the provider's stable account id
	Email         string
	EmailVerified bool
	Login         string // GitHub login; empty for other providers
}

var ErrEmailUnverified = httpx.New(http.StatusForbidden, "email_unverified", "Your account has no verified email address")

// SignIn finds or creates the user behind an external identity and opens a
// session. A known (provider, subject) is that user. A new one is linked to
// the user with the same verified email, or creates one; without a verified
// email it is refused, or anyone could claim an account by typing its email
// at the provider. Concurrent first sign-ins of the same identity converge
// on one user through the ON CONFLICT clauses.
func (s *Service) SignIn(ctx context.Context, id Identity) (User, string, error) {
	email := NormalizeEmail(id.Email)
	if id.Provider == "" || id.Subject == "" {
		return User{}, "", errors.New("identity: provider and subject are required")
	}
	token, sid, err := newSessionToken()
	if err != nil {
		return User{}, "", err
	}
	var u User
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var userID string
		err := tx.QueryRow(ctx, `UPDATE user_identities SET email = $3, login = $4, last_login_at = now()
			WHERE provider = $1 AND subject = $2 RETURNING user_id`, id.Provider, id.Subject, email, id.Login).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			userID, err = s.attachIdentity(ctx, tx, id, email)
		}
		if err != nil {
			return err
		}
		if err := scanUser(tx.QueryRow(ctx, `UPDATE users SET role = CASE WHEN email = ANY($2) THEN 'admin' ELSE 'user' END
			WHERE id = $1 RETURNING `+userColumns, userID, s.adminList()), &u); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, now() + $3::interval)`, sid, u.ID, sessionTTL.String())
		return err
	})
	if err != nil {
		return User{}, "", err
	}
	return u, token, nil
}

// attachIdentity records a first sign-in of id: to the user with its email,
// or to a new user.
func (s *Service) attachIdentity(ctx context.Context, tx pgx.Tx, id Identity, email string) (string, error) {
	if !id.EmailVerified || email == "" {
		return "", ErrEmailUnverified
	}
	created, err := tx.Exec(ctx, `INSERT INTO users (id, email, role) VALUES ($1, $2, $3) ON CONFLICT (email) DO NOTHING`,
		idgen.New("user"), email, s.roleFor(email))
	if err != nil {
		return "", err
	}
	var userID string
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID); err != nil {
		return "", err
	}
	linked, err := tx.Exec(ctx, `INSERT INTO user_identities (provider, subject, user_id, email, login) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (provider, subject) DO NOTHING`, id.Provider, id.Subject, userID, email, id.Login)
	if err != nil {
		return "", err
	}
	if linked.RowsAffected() == 0 {
		return userID, nil // a concurrent first sign-in of the same identity got there first
	}
	action := "user.identity_linked"
	if created.RowsAffected() == 1 {
		action = "user.signed_up"
	}
	return userID, audit.Record(ctx, tx, audit.Event{ActorID: userID, ActorKind: KindUser, Action: action,
		AggregateKind: "user", AggregateID: userID, RequestID: httpx.RequestID(ctx),
		Payload: map[string]string{"provider": id.Provider}})
}

func (s *Service) adminList() []string {
	out := make([]string, 0, len(s.adminEmails))
	for e := range s.adminEmails {
		out = append(out, e)
	}
	return out
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
