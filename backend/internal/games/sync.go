package games

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/rating"
	"tolerance/internal/games/tanks/house"
	"tolerance/internal/platform/db"
)

// Sync upserts the house bots. Idempotent; run by cmd/migrate on every start.
func Sync(ctx context.Context, pool *db.Pool) error {
	return syncHouseBots(ctx, pool)
}

// syncHouseBots upserts one game_bots row and one active bot_versions row per house.Names() entry, with
// fixed ids ("bot_house_<name>", "bv_house_<name>") so re-running Sync is a no-op once they exist.
func syncHouseBots(ctx context.Context, pool *db.Pool) error {
	return pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, name := range house.Names() {
			botID := "bot_house_" + name
			versionID := "bv_house_" + name
			if _, err := tx.Exec(ctx, `
				INSERT INTO game_bots (id, game, house, name, mu, sigma)
				VALUES ($1, $2, true, $3, $4, $5)
				ON CONFLICT (id) DO UPDATE SET name = $3`,
				botID, Game, name, rating.DefaultMu, rating.DefaultSigma); err != nil {
				return fmt.Errorf("games: sync house bot %s: %w", name, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO bot_versions (id, bot_id, number, source, language, entry, status)
				VALUES ($1, $2, 1, 'house', 'builtin', $3, 'active')
				ON CONFLICT (id) DO NOTHING`,
				versionID, botID, name); err != nil {
				return fmt.Errorf("games: sync house version %s: %w", name, err)
			}
			if _, err := tx.Exec(ctx, `UPDATE game_bots SET active_version_id = $2 WHERE id = $1 AND active_version_id IS DISTINCT FROM $2`,
				botID, versionID); err != nil {
				return fmt.Errorf("games: activate house version %s: %w", name, err)
			}
		}
		return nil
	})
}
