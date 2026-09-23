package agents

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Heartbeat records that the connector is alive. The row is rewritten at
// most every 10 seconds so a chatty connector does not turn into writes.
func (s *Service) Heartbeat(ctx context.Context, agentID, version, hostname string) error {
	if len(version) > 40 {
		version = version[:40]
	}
	if len(hostname) > 80 {
		hostname = hostname[:80]
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO agent_presence (agent_id, last_seen_at, connector_version, hostname) VALUES ($1, now(), $2, $3)
			ON CONFLICT (agent_id) DO UPDATE SET last_seen_at = now(), connector_version = $2, hostname = $3
			WHERE agent_presence.last_seen_at < now() - interval '10 seconds'`, agentID, version, hostname)
		return err
	})
}

func (s *Service) presence(ctx context.Context, tx pgx.Tx, agentID string) (*Presence, error) {
	var p Presence
	err := tx.QueryRow(ctx, `SELECT last_seen_at, connector_version, hostname FROM agent_presence WHERE agent_id = $1`, agentID).
		Scan(&p.LastSeenAt, &p.ConnectorVersion, &p.Hostname)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.LastSeenAt = p.LastSeenAt.UTC()
	return &p, nil
}

func (s *Service) overview(ctx context.Context, a Agent) (Overview, error) {
	p, err := s.private(ctx, a)
	if err != nil {
		return Overview{}, err
	}
	var pr *Presence
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		pr, err = s.presence(ctx, tx, a.ID)
		return err
	})
	if err != nil {
		return Overview{}, err
	}
	facts, err := s.proofs.ProofFacts(ctx, a.ID)
	if err != nil {
		return Overview{}, err
	}
	var seen *time.Time
	if pr != nil {
		seen = &pr.LastSeenAt
	}
	return Overview{Private: p, Presence: pr, Stage: ComputeStage(len(p.APIKeys) > 0, seen, time.Now(), facts)}, nil
}

// Overview returns nil, nil when the user has no agent yet.
func (s *Service) Overview(ctx context.Context, userID string) (*Overview, error) {
	a, err := s.byOwner(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o, err := s.overview(ctx, a)
	return &o, err
}

func (s *Service) OverviewByID(ctx context.Context, agentID string) (Overview, error) {
	a, err := s.ByID(ctx, agentID)
	if err != nil {
		return Overview{}, err
	}
	return s.overview(ctx, a)
}
