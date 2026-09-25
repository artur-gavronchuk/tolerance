package games

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/idgen"
)

// RefreshBroadcast decides what the arena's live page shows next, under a transaction-scoped advisory
// lock. If the current broadcast (if any) is still airing, it does nothing. Otherwise it looks for the
// best finished ladder match from the last 30 minutes that hasn't been broadcast yet - highest combined
// post-match rating, ties broken by total kills - and schedules it to start in 3 seconds; with no such
// match, it repeats the last one shown (if there was ever one at all).
func (s *Service) RefreshBroadcast(ctx context.Context) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('games:broadcast'))`); err != nil {
			return err
		}

		var lastMatchID string
		var startsAt time.Time
		var durationMS int
		err := tx.QueryRow(ctx, `SELECT match_id, starts_at, duration_ms FROM tanks_broadcasts ORDER BY starts_at DESC LIMIT 1`).
			Scan(&lastMatchID, &startsAt, &durationMS)
		hasLast := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if hasLast && startsAt.Add(time.Duration(durationMS)*time.Millisecond).After(time.Now()) {
			return nil // still airing
		}

		var candidateID string
		var ticks int
		err = tx.QueryRow(ctx, `
			SELECT m.id, m.ticks
			FROM matches m
			JOIN match_players mp ON mp.match_id = m.id
			WHERE m.kind = 'ladder' AND m.status = 'finished' AND m.finished_at > now() - interval '30 minutes'
			  AND NOT EXISTS (SELECT 1 FROM tanks_broadcasts b WHERE b.match_id = m.id)
			GROUP BY m.id
			ORDER BY sum(1000 + 40 * (mp.mu_after - 3 * mp.sigma_after)) DESC, sum(mp.kills) DESC
			LIMIT 1`).Scan(&candidateID, &ticks)
		switch {
		case err == nil:
			if _, err := tx.Exec(ctx, `UPDATE matches SET featured = true WHERE id = $1`, candidateID); err != nil {
				return err
			}
			return insertBroadcast(ctx, tx, candidateID, ticks*100)
		case errors.Is(err, pgx.ErrNoRows):
			if !hasLast {
				return nil
			}
			return insertBroadcast(ctx, tx, lastMatchID, durationMS)
		default:
			return err
		}
	})
}

func insertBroadcast(ctx context.Context, tx pgx.Tx, matchID string, durationMS int) error {
	id := idgen.New("bcast")
	_, err := tx.Exec(ctx, `INSERT INTO tanks_broadcasts (id, match_id, starts_at, duration_ms)
		VALUES ($1, $2, now() + interval '3 seconds', $3)`, id, matchID, durationMS)
	return err
}

// Live returns the broadcast schedule currently in effect (the last row written by RefreshBroadcast) plus
// the server's own clock, so a client can time its countdown against starts_at correctly even with a
// skewed local clock. MatchID is nil if nothing has ever been broadcast.
func (s *Service) Live(ctx context.Context) (LiveView, error) {
	var out LiveView
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var matchID string
		var startsAt time.Time
		var durationMS int
		err := tx.QueryRow(ctx, `SELECT match_id, starts_at, duration_ms FROM tanks_broadcasts ORDER BY starts_at DESC LIMIT 1`).
			Scan(&matchID, &startsAt, &durationMS)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		out.MatchID = &matchID
		st := startsAt.UTC()
		out.StartsAt = &st
		out.DurationMS = durationMS
		return nil
	})
	out.Now = time.Now().UTC()
	if err != nil {
		return LiveView{}, err
	}
	return out, nil
}
