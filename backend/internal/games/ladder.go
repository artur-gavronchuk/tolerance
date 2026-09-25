package games

import (
	"context"
	"math/rand/v2"
	"os"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/rating"
	"tolerance/internal/games/tanks"
	"tolerance/internal/games/tanks/house"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/jobs"
)

// ladderBot is one bot's current standing, as picked for scheduling a ladder match.
type ladderBot struct {
	BotID, VersionID string
	Name             string
	Mu, Sigma        float64
	LastMatchAt      *time.Time
}

// houseOnlyGap is how long the ladder waits between matches made entirely of house bots, so an idle
// tournament (nobody has an active bot yet) doesn't spam matches nobody is watching. It is stricter than
// the ordinary ARENA_MATCH_INTERVAL, which still applies on top of it.
const houseOnlyGap = 2 * time.Minute

// ScheduleTick creates at most one new ladder match, under a transaction-scoped advisory lock so
// concurrent schedulers never race each other. It creates nothing when concurrency open matches already
// exist, when less than interval has passed since the last ladder match was created, or - when there are
// no active user bots yet - when less than houseOnlyGap has passed. Returns the new match's id, or "".
func (s *Service) ScheduleTick(ctx context.Context, concurrency int, interval time.Duration) (string, error) {
	var matchID string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('games:schedule'))`); err != nil {
			return err
		}

		var open int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM matches WHERE status IN ('queued', 'running')`).Scan(&open); err != nil {
			return err
		}
		if open >= concurrency {
			return nil
		}

		var lastCreated *time.Time
		if err := tx.QueryRow(ctx, `SELECT max(created_at) FROM matches WHERE kind = 'ladder'`).Scan(&lastCreated); err != nil {
			return err
		}
		if lastCreated != nil && time.Since(*lastCreated) < interval {
			return nil
		}

		userBots, err := s.activeUserBots(ctx, tx)
		if err != nil {
			return err
		}

		rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(os.Getpid())))

		var players []ladderBot
		if len(userBots) == 0 {
			if lastCreated != nil && time.Since(*lastCreated) < houseOnlyGap {
				return nil
			}
			houseBots, err := s.houseLadderBots(ctx, tx)
			if err != nil {
				return err
			}
			if len(houseBots) < 2 {
				return nil
			}
			players = houseBots
		} else {
			players, err = s.pickLeadAndOpponents(ctx, tx, userBots, rng)
			if err != nil {
				return err
			}
		}
		rng.Shuffle(len(players), func(i, j int) { players[i], players[j] = players[j], players[i] })

		id, err := s.createLadderMatch(ctx, tx, rng, players)
		if err != nil {
			return err
		}
		matchID = id
		return nil
	})
	return matchID, err
}

func scanLadderBots(rows pgx.Rows) ([]ladderBot, error) {
	var out []ladderBot
	for rows.Next() {
		var b ladderBot
		var lastMatchAt *time.Time
		if err := rows.Scan(&b.BotID, &b.Name, &b.Mu, &b.Sigma, &b.VersionID, &lastMatchAt); err != nil {
			return nil, err
		}
		if lastMatchAt != nil {
			u := lastMatchAt.UTC()
			b.LastMatchAt = &u
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// activeUserBots lists every owner bot (not house) with an active version.
func (s *Service) activeUserBots(ctx context.Context, tx pgx.Tx) ([]ladderBot, error) {
	rows, err := tx.Query(ctx, `SELECT id, name, mu, sigma, active_version_id, last_match_at
		FROM game_bots WHERE house = false AND active_version_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLadderBots(rows)
}

// houseLadderBots loads the house.Ladder bots (hunter, sniper - not idle, which never plays in the ladder).
func (s *Service) houseLadderBots(ctx context.Context, tx pgx.Tx) ([]ladderBot, error) {
	ids := make([]string, len(house.Ladder))
	for i, name := range house.Ladder {
		ids[i] = "bot_house_" + name
	}
	rows, err := tx.Query(ctx, `SELECT id, name, mu, sigma, active_version_id, last_match_at
		FROM game_bots WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLadderBots(rows)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// pickLeadAndOpponents picks the ladder match's lead (the user bot with the oldest last_match_at, NULL
// first, ties broken by larger sigma) plus up to three opponents drawn from the six other active bots
// (other user bots and the house ladder bots) nearest the lead's displayed rating, shuffled with rng and
// topped up with any as-yet-unused house.Ladder bot if that pool came up short.
func (s *Service) pickLeadAndOpponents(ctx context.Context, tx pgx.Tx, userBots []ladderBot, rng *rand.Rand) ([]ladderBot, error) {
	sort.Slice(userBots, func(i, j int) bool {
		li, lj := userBots[i].LastMatchAt, userBots[j].LastMatchAt
		if (li == nil) != (lj == nil) {
			return li == nil
		}
		if li != nil && lj != nil && !li.Equal(*lj) {
			return li.Before(*lj)
		}
		return userBots[i].Sigma > userBots[j].Sigma
	})
	lead := userBots[0]
	leadRating := rating.Display(rating.Rating{Mu: lead.Mu, Sigma: lead.Sigma})

	houseBots, err := s.houseLadderBots(ctx, tx)
	if err != nil {
		return nil, err
	}
	pool := append(append([]ladderBot{}, userBots[1:]...), houseBots...)

	sort.Slice(pool, func(i, j int) bool {
		di := absInt(rating.Display(rating.Rating{Mu: pool[i].Mu, Sigma: pool[i].Sigma}) - leadRating)
		dj := absInt(rating.Display(rating.Rating{Mu: pool[j].Mu, Sigma: pool[j].Sigma}) - leadRating)
		return di < dj
	})
	if len(pool) > 6 {
		pool = pool[:6]
	}
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if len(pool) > 3 {
		pool = pool[:3]
	}

	if len(pool) < 3 {
		have := map[string]bool{lead.BotID: true}
		for _, p := range pool {
			have[p.BotID] = true
		}
		for _, hb := range houseBots {
			if len(pool) >= 3 {
				break
			}
			if !have[hb.BotID] {
				pool = append(pool, hb)
				have[hb.BotID] = true
			}
		}
	}

	return append([]ladderBot{lead}, pool...), nil
}

// createLadderMatch inserts a queued ladder match with players in the given order, and enqueues its
// run_match job.
func (s *Service) createLadderMatch(ctx context.Context, tx pgx.Tx, rng *rand.Rand, players []ladderBot) (string, error) {
	seed := rng.Int64N(1 << 53)
	m := tanks.PickMap(seed)
	ticks := tanks.DefaultRules().Ticks
	id := idgen.New("match")
	if _, err := tx.Exec(ctx, `INSERT INTO matches (id, game, kind, status, seed, map, ticks) VALUES ($1, $2, 'ladder', 'queued', $3, $4, $5)`,
		id, Game, seed, m.Name, ticks); err != nil {
		return "", err
	}
	for i, p := range players {
		if _, err := tx.Exec(ctx, `INSERT INTO match_players (match_id, slot, bot_id, version_id) VALUES ($1, $2, $3, $4)`,
			id, i, p.BotID, p.VersionID); err != nil {
			return "", err
		}
	}
	if _, err := jobs.Enqueue(ctx, tx, "run_match", map[string]string{"match_id": id}, "match:"+id); err != nil {
		return "", err
	}
	return id, nil
}
