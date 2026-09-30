package agents

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/idgen"
)

type Version struct {
	ID           string    `json:"id"`
	Number       int       `json:"number"`
	Model        string    `json:"model"`
	Harness      string    `json:"harness"`
	ConfigDigest string    `json:"config_digest"`
	CreatedAt    time.Time `json:"created_at"`
}

type VersionInput struct {
	Model        string `json:"model"`
	Harness      string `json:"harness"`
	ConfigDigest string `json:"config_digest"`
}

// VersionListener is told, inside the same transaction, that an agent got
// a new version. The ratings module uses it to reset confidence.
type VersionListener interface {
	OnNewVersion(ctx context.Context, tx pgx.Tx, agentID, versionID string) error
}

func (s *Service) SetVersionListener(l VersionListener) { s.versions = l }

const versionCols = `id, number, model, harness, config_digest, created_at`

func scanVersion(row interface{ Scan(...any) error }, v *Version) error {
	if err := row.Scan(&v.ID, &v.Number, &v.Model, &v.Harness, &v.ConfigDigest, &v.CreatedAt); err != nil {
		return err
	}
	v.CreatedAt = v.CreatedAt.UTC()
	return nil
}

// EnsureVersion returns the version for this digest, creating the next
// numbered one when the digest is new, and makes it current. The listener
// hears about every change of the current version, including a switch back
// to a digest seen before (created is false then).
func (s *Service) EnsureVersion(ctx context.Context, agentID string, in VersionInput) (Version, bool, error) {
	if in.ConfigDigest == "" {
		return Version{}, false, errors.New("agents: config digest is required")
	}
	if len(in.Model) > 80 {
		in.Model = in.Model[:80]
	}
	if len(in.Harness) > 80 {
		in.Harness = in.Harness[:80]
	}
	var v Version
	created := false
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var current *string
		if err := tx.QueryRow(ctx, `SELECT current_version_id FROM agents WHERE id = $1 FOR UPDATE`, agentID).Scan(&current); err != nil {
			return err
		}
		err := scanVersion(tx.QueryRow(ctx, `SELECT `+versionCols+` FROM agent_versions WHERE agent_id = $1 AND config_digest = $2`, agentID, in.ConfigDigest), &v)
		switch {
		case err == nil && current != nil && *current == v.ID:
			return nil // the common heartbeat: nothing changed
		case err == nil:
			// A digest seen before, not current: the owner switched back.
		case errors.Is(err, pgx.ErrNoRows):
			if err := scanVersion(tx.QueryRow(ctx, `INSERT INTO agent_versions (id, agent_id, number, model, harness, config_digest)
				VALUES ($1, $2, (SELECT coalesce(max(number), 0) + 1 FROM agent_versions WHERE agent_id = $2), $3, $4, $5)
				RETURNING `+versionCols, idgen.New("ver"), agentID, in.Model, in.Harness, in.ConfigDigest), &v); err != nil {
				return err
			}
			created = true
		default:
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE agents SET current_version_id = $2 WHERE id = $1`, agentID, v.ID); err != nil {
			return err
		}
		if s.versions != nil {
			return s.versions.OnNewVersion(ctx, tx, agentID, v.ID)
		}
		return nil
	})
	return v, created, err
}

func (s *Service) CurrentVersion(ctx context.Context, agentID string) (*Version, error) {
	var v Version
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanVersion(tx.QueryRow(ctx, `SELECT v.id, v.number, v.model, v.harness, v.config_digest, v.created_at FROM agent_versions v JOIN agents a ON a.current_version_id = v.id WHERE a.id = $1`, agentID), &v)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}
