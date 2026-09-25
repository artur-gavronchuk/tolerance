package games

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/match"
	"tolerance/internal/games/rating"
	"tolerance/internal/games/tanks"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

// Qualify runs the four automated checks on a pending version, stores them (with the check match and its
// replay) and either activates the version or rejects it. The check match is 1v1 against the house idle
// bot only (not hunter): an aggressive third player made the outcome depend heavily on spawn geometry (an
// unmodified starter kit passed only about 70% of the time), which would reject perfectly good bots at
// random - a bot's behaviour under fire is what the ladder itself shows, not this check.
//
// A version that is not pending is left alone: its already-stored result is returned instead of re-running
// anything, which makes Qualify idempotent. err is non-nil only for a platform failure (the job that
// called this will retry); every verdict about the bot itself is recorded and returned with err == nil.
//
// Activation is decided under game_bots's row lock (not from a snapshot read before the check match runs):
// two pending versions of the same bot can be qualified concurrently (two worker goroutines, or two API
// replicas), and deciding from a pre-match snapshot of active_version_id would let an older version's
// activation land after a newer one's and silently move active_version_id backwards. Locking game_bots in
// the final transaction (after the slow part - running the match - is already done) means whichever
// version's Qualify call reaches that transaction second sees the first one's committed result and is
// correctly superseded instead of overwriting it. See VersionView's doc comment for what 'active' means on
// a bot_versions row versus on the bot itself.
func (s *Service) Qualify(ctx context.Context, versionID string) (bool, []Check, error) {
	var v struct {
		botID, source, language, entry, status string
		number                                 int
		archive, checksRaw                     []byte
	}
	var botName string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT bv.bot_id, bv.number, bv.source, bv.language, bv.entry, bv.archive, bv.status, bv.checks, gb.name
			FROM bot_versions bv
			JOIN game_bots gb ON gb.id = bv.bot_id
			WHERE bv.id = $1`, versionID).
			Scan(&v.botID, &v.number, &v.source, &v.language, &v.entry, &v.archive, &v.status, &v.checksRaw, &botName)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, httpx.NotFound()
	}
	if err != nil {
		return false, nil, err
	}

	if v.status != "pending" {
		var checks []Check
		if len(v.checksRaw) > 0 {
			if err := json.Unmarshal(v.checksRaw, &checks); err != nil {
				return false, nil, err
			}
		}
		return v.status == "active", checks, nil
	}

	idle, err := s.houseParticipant(ctx, "idle")
	if err != nil {
		return false, nil, err
	}
	candidate := playerInput{
		BotID: v.botID, VersionID: versionID, Name: botName, Source: v.source, Number: v.number,
		Language: v.language, Entry: v.entry, Archive: v.archive,
	}
	participants := []playerInput{candidate, idle}

	players, cleanup, err := s.launchSpecs(participants)
	defer cleanup()
	if err != nil {
		return false, nil, err
	}

	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(os.Getpid())))
	seed := rng.Int64N(1 << 53)
	// bunkers's per-spawn L-shaped walls can trap a bot that has no obstacle avoidance - just turning and
	// driving straight at the target's angle, like both starter kits ship - so it never even reaches idle:
	// measured against an unmodified starter kit, arena and crossroads resolved every one of 40 sampled
	// seeds outright, while bunkers tied (never engaged at all) on all 40. A tie shares first place with
	// idle and fails beats_idle, which would reject an otherwise perfectly competent bot purely for
	// landing on this map. Nudge to the next map bucket instead (tanks.Maps() is [arena, crossroads,
	// bunkers], so seed+1 always lands on arena from a bunkers seed). The ladder itself still plays
	// bunkers - this only keeps it out of the qualification gate.
	if tanks.PickMap(seed).Name == "bunkers" {
		seed++
	}
	m := tanks.PickMap(seed)

	result, err := match.Run(ctx, s.l, match.Config{Seed: seed, Map: m.Name, Ticks: s.cfg.CheckTicks}, players)
	if err != nil {
		return false, nil, err
	}

	checks := buildChecks(result)
	qualified := checksPassed(checks)
	checkLog := result.Players[0].Stderr

	matchID, err := s.storeCheckMatch(ctx, seed, m.Name, participants, result)
	if err != nil {
		return false, nil, err
	}

	var status string
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var activeVersionID *string
		var mu, sigma float64
		if err := tx.QueryRow(ctx, `SELECT active_version_id, mu, sigma FROM game_bots WHERE id = $1 FOR UPDATE`, v.botID).
			Scan(&activeVersionID, &mu, &sigma); err != nil {
			return err
		}
		var activeNumber *int
		if activeVersionID != nil {
			if err := tx.QueryRow(ctx, `SELECT number FROM bot_versions WHERE id = $1`, *activeVersionID).Scan(&activeNumber); err != nil {
				return err
			}
		}

		switch {
		case !qualified:
			status = "rejected"
		case activeNumber == nil || v.number > *activeNumber:
			status = "active"
		default:
			status = "rejected"
			checks = append(checks, Check{Name: "supersede", Passed: false,
				Detail: fmt.Sprintf("superseded by version %d", *activeNumber)})
		}

		checksJSON, err := json.Marshal(checks)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE bot_versions SET status = $2, checks = $3, check_log = $4, check_match_id = $5
			WHERE id = $1 AND status = 'pending'`,
			versionID, status, checksJSON, checkLog, matchID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // lost a race with another Qualify run on this version; leave it as that run left it
		}
		if status == "active" {
			refreshed := rating.Refresh(rating.Rating{Mu: mu, Sigma: sigma})
			if _, err := tx.Exec(ctx, `UPDATE game_bots SET active_version_id = $2, sigma = $3 WHERE id = $1`,
				v.botID, versionID, refreshed.Sigma); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, nil, err
	}
	return status == "active", checks, nil
}

// houseParticipant loads name's house bot (its id, current active version and that version's number) as a
// playerInput ready for launchSpecs.
func (s *Service) houseParticipant(ctx context.Context, name string) (playerInput, error) {
	botID := "bot_house_" + name
	p := playerInput{BotID: botID, Name: name, House: true, Source: "house", Entry: name}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT v.id, v.number FROM game_bots g JOIN bot_versions v ON v.id = g.active_version_id
			WHERE g.id = $1`, botID).Scan(&p.VersionID, &p.Number)
	})
	if err != nil {
		return playerInput{}, fmt.Errorf("games: house participant %s: %w", name, err)
	}
	return p, nil
}

// buildChecks derives the four qualification checks from a check match's result. result.Players[0] is the
// candidate, result.Players[1] is the house idle bot it must beat. A check after the first failure is
// recorded as skipped rather than evaluated, per the qualification design.
func buildChecks(result match.Result) []Check {
	c := result.Players[0]
	idle := result.Players[1]

	checks := []Check{{Name: "package", Passed: true, Detail: "the archive unpacked and the bot started"}}

	startsDetail := "the bot answered ready in time"
	if !c.Ready {
		startsDetail = "the bot never answered ready"
	}
	checks = append(checks, Check{Name: "starts", Passed: c.Ready, Detail: startsDetail})
	if !c.Ready {
		return append(checks,
			Check{Name: "stable", Passed: false, Detail: "skipped"},
			Check{Name: "beats_idle", Passed: false, Detail: "skipped"})
	}

	stablePassed := c.Status != match.StatusCrashed && c.Status != match.StatusTimeout && c.Status != match.StatusInvalid &&
		float64(c.Answered) >= 0.95*float64(c.Asked)
	detail := fmt.Sprintf("answered %d of %d ticks", c.Answered, c.Asked)
	if c.Noise > 0 {
		detail += fmt.Sprintf("; printed %d non-command lines to stdout; write logs to stderr", c.Noise)
	}
	checks = append(checks, Check{Name: "stable", Passed: stablePassed, Detail: detail})
	if !stablePassed {
		return append(checks, Check{Name: "beats_idle", Passed: false, Detail: "skipped"})
	}

	beatsIdle := c.Place < idle.Place
	return append(checks, Check{Name: "beats_idle", Passed: beatsIdle,
		Detail: fmt.Sprintf("placed %d, idle placed %d", c.Place, idle.Place)})
}

func checksPassed(checks []Check) bool {
	for _, c := range checks {
		if !c.Passed {
			return false
		}
	}
	return true
}

// storeCheckMatch records a finished kind='check' match (with its replay) for a qualification run and
// returns its id.
func (s *Service) storeCheckMatch(ctx context.Context, seed int64, mapName string, participants []playerInput, result match.Result) (string, error) {
	matchID := idgen.New("match")
	playedTicks := len(result.Replay.Frames) - 1
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO matches (id, game, kind, status, seed, map, ticks, started_at, finished_at)
			VALUES ($1, $2, 'check', 'finished', $3, $4, $5, now(), now())`,
			matchID, Game, seed, mapName, playedTicks); err != nil {
			return err
		}
		for i, p := range participants {
			o := result.Players[i]
			if _, err := tx.Exec(ctx, `INSERT INTO match_players
				(match_id, slot, bot_id, version_id, place, kills, damage, death_tick, status, answered, asked, noise, stderr_tail)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
				matchID, i, p.BotID, p.VersionID, o.Place, o.Kills, o.Damage, o.DeathTick, o.Status, o.Answered, o.Asked, o.Noise, o.Stderr); err != nil {
				return err
			}
		}
		return insertReplay(ctx, tx, matchID, participants, result)
	})
	return matchID, err
}

// markCheckPlatformFailure is called once a check_bot job has used every retry: the platform, not the bot,
// is at fault, but a pending version can't be left pending forever, so it is rejected with a single check
// naming the problem and its owner has to upload again.
func (s *Service) markCheckPlatformFailure(ctx context.Context, versionID string) error {
	checks := []Check{{Name: "platform", Passed: false, Detail: "the platform could not run the check; upload again"}}
	checksJSON, err := json.Marshal(checks)
	if err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE bot_versions SET status = 'rejected', checks = $2 WHERE id = $1 AND status = 'pending'`,
			versionID, checksJSON)
		return err
	})
}

// sweepFailedChecks is markCheckPlatformFailure's periodic counterpart, for the one path that never goes
// through the games worker's own final-attempt handling in worker.go: jobs.Queue.Reclaim parks a job as
// failed once its lease expires and every attempt is used, but Reclaim itself is generic (it has no idea
// what a check_bot job means) and is only ever called from internal/proofs.Worker's maintenance sweep,
// against the whole shared jobs table - not from anything in this package. A check_bot job that dies that
// way (its owning process crashed mid-check, rather than returning an error worker.handle could catch)
// would otherwise leave its version stuck 'pending' forever. Called from the games worker's own periodic
// loop alongside SweepStuck; returns how many versions it rejected.
func (s *Service) sweepFailedChecks(ctx context.Context) (int, error) {
	checks := []Check{{Name: "platform", Passed: false, Detail: "the platform could not run the check; upload again"}}
	checksJSON, err := json.Marshal(checks)
	if err != nil {
		return 0, err
	}
	var n int64
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE bot_versions SET status = 'rejected', checks = $1
			WHERE status = 'pending' AND id IN (
				SELECT payload ->> 'version_id' FROM jobs WHERE kind = 'check_bot' AND state = 'failed'
			)`, checksJSON)
		n = tag.RowsAffected()
		return err
	})
	return int(n), err
}
