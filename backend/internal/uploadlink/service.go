// Package uploadlink is the personal upload link: a secret token in a URL that lets a person's coding agent
// download tasks and upload results as that person, without a session. The token only reaches the upload and
// status routes below; it never opens anything else.
package uploadlink

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
	"tolerance/internal/analytics"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
)

// ErrNoLink means the token matches no active link.
var ErrNoLink = errors.New("uploadlink: no such link")

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

// Status is the owner's view of their link; the token itself is never stored, so it is shown once.
type Status struct {
	Active     bool       `json:"active"`
	CreatedAt  *time.Time `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (s *Service) Status(ctx context.Context, userID string) (Status, error) {
	var st Status
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT true, created_at, last_used_at FROM upload_links WHERE user_id = $1`, userID).
			Scan(&st.Active, &st.CreatedAt, &st.LastUsedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, nil
	}
	st.CreatedAt, st.LastUsedAt = utc(st.CreatedAt), utc(st.LastUsedAt)
	return st, err
}

// Rotate creates the person's link or replaces it (the old URL stops working at once) and returns the new token.
func (s *Service) Rotate(ctx context.Context, userID string) (token string, st Status, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", Status{}, err
	}
	token = "ul_" + hex.EncodeToString(b[:])
	defer func() {
		if err == nil {
			analytics.Track("upload_link.created", userID, nil)
		}
	}()
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO upload_links (user_id, token_hash) VALUES ($1, $2)
			ON CONFLICT (user_id) DO UPDATE SET token_hash = EXCLUDED.token_hash, created_at = now(), last_used_at = NULL
			RETURNING true, created_at, last_used_at`, userID, hash(token)).Scan(&st.Active, &st.CreatedAt, &st.LastUsedAt)
	})
	st.CreatedAt, st.LastUsedAt = utc(st.CreatedAt), utc(st.LastUsedAt)
	return token, st, err
}

func (s *Service) Revoke(ctx context.Context, userID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM upload_links WHERE user_id = $1`, userID)
		return err
	})
}

// UserByToken resolves a token to its owner's id; last_used_at is bumped at most once a minute.
func (s *Service) UserByToken(ctx context.Context, token string) (string, error) {
	var uid string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE upload_links l SET last_used_at = CASE WHEN last_used_at IS NULL OR last_used_at < now() - interval '1 minute' THEN now() ELSE last_used_at END
			FROM users u WHERE l.token_hash = $1 AND u.id = l.user_id AND u.banned_at IS NULL RETURNING l.user_id`, hash(token)).Scan(&uid)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoLink
	}
	return uid, err
}

// Key is a stable, non-secret identifier of a token for rate limiting and logs.
func Key(token string) string { return hash(token)[:16] }
