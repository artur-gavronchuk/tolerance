package games

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/rating"
	"tolerance/internal/games/tanks"
	"tolerance/internal/games/tanks/house"
	"tolerance/internal/platform/db"
	"tolerance/internal/proofs"
)

// tanksBotSlug is the proof task an agent solves to improve its owner's tanks bot.
const tanksBotSlug = "tanks-bot"

// Sync upserts the house bots and the tanks-bot proof task. Idempotent; run by cmd/migrate on every start,
// so a fresh database (or one with a changed starter kit / task text) always ends up with both in place.
func Sync(ctx context.Context, pool *db.Pool) error {
	if err := syncHouseBots(ctx, pool); err != nil {
		return err
	}
	return syncTanksBotTask(ctx, pool)
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

// syncTanksBotTask upserts the "tanks-bot" proof task: a kind = game_bot task whose repo is the Python
// starter kit plus GAME.md and whose hidden tarball is empty - there is nothing to run in the sandbox for
// it, since the agent's diff becomes a bot version rather than being graded against hidden tests.
func syncTanksBotTask(ctx context.Context, pool *db.Pool) error {
	files, err := tanks.Starter("python")
	if err != nil {
		return fmt.Errorf("games: tanks starter: %w", err)
	}
	repoTar, err := proofs.TarFiles(files)
	if err != nil {
		return fmt.Errorf("games: pack tanks-bot repo: %w", err)
	}
	hiddenTar, err := proofs.TarFiles(nil)
	if err != nil {
		return fmt.Errorf("games: pack tanks-bot hidden: %w", err)
	}
	sum := sha256.Sum256(repoTar)

	task := proofs.Task{
		Slug:            tanksBotSlug,
		Title:           "Improve your tanks bot",
		Language:        "python",
		Kind:            proofs.KindGameBot,
		Image:           "arena-tanks-bot:1",
		RunCmd:          "true",
		AgentTimeoutS:   1200,
		SandboxTimeoutS: 120,
		TaskMD:          tanks.AgentTaskMD,
		RepoTar:         repoTar,
		HiddenTar:       hiddenTar,
		RepoSHA256:      hex.EncodeToString(sum[:]),
	}
	if err := proofs.SyncCatalog(ctx, pool, []proofs.Task{task}); err != nil {
		return fmt.Errorf("games: sync tanks-bot task: %w", err)
	}
	return nil
}
