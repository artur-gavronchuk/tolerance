package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
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
