package games

import (
	"context"
	"errors"
	"os"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/botpkg"
	"tolerance/internal/games/match"
	"tolerance/internal/games/rating"
	"tolerance/internal/games/tanks"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/metrics"
	"tolerance/internal/platform/sanitize"
)

// playerInput is everything needed to both launch one bot for a match (via launchSpecs) and record it
// afterwards (bot id, version id, its number/source/house for the replay header and match_players).
type playerInput struct {
	BotID, VersionID string
	Name             string
	House            bool
	Source           string
	Number           int
	Language, Entry  string
	Archive          []byte
}

// launchSpecs resolves participants into match.Player values in the same order, unpacking any non-house
// archive into a fresh temporary directory under s.cfg.WorkDir. cleanup removes every directory it
// created and must always be called, whether or not err is nil - a later participant's failure must not
// leak an earlier one's directory.
func (s *Service) launchSpecs(participants []playerInput) (players []match.Player, cleanup func(), err error) {
	var dirs []string
	cleanup = func() {
		for _, d := range dirs {
			os.RemoveAll(d)
		}
	}
	players = make([]match.Player, len(participants))
	for i, p := range participants {
		if p.House {
			players[i] = match.Player{Name: p.Name, Spec: match.Spec{House: p.Entry}}
			continue
		}
		dir, err := os.MkdirTemp(s.cfg.WorkDir, "bot-")
		if err != nil {
			return nil, cleanup, err
		}
		dirs = append(dirs, dir)
		if err := botpkg.Unpack(p.Archive, dir); err != nil {
			return nil, cleanup, err
		}
		players[i] = match.Player{Name: p.Name, Spec: match.Spec{Dir: dir, Language: p.Language, Entry: p.Entry}}
	}
	return players, cleanup, nil
}

// insertReplay encodes result.Replay (with Players filled in from participants) and stores it, replacing
// any replay the match already had.
func insertReplay(ctx context.Context, tx pgx.Tx, matchID string, participants []playerInput, result match.Result) error {
	replay := result.Replay
	for i, p := range participants {
		replay.Players[i].BotID = p.BotID
		replay.Players[i].Version = p.Number
		replay.Players[i].House = p.House
		replay.Players[i].Source = p.Source
	}
	data, err := tanks.EncodeReplay(replay)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO match_replays (match_id, data) VALUES ($1, $2)
		ON CONFLICT (match_id) DO UPDATE SET data = EXCLUDED.data, created_at = now()`, matchID, data)
	return err
}

// RunMatch plays one queued (or already-running) match end to end. It claims the match by moving it to
// running regardless of whether it was already running - a server restart mid-match leaves a match wedged
// in running with no live worker behind it, and the next claim must redo the whole thing from scratch
// rather than get stuck forever. err != nil only for a platform failure (the run_match job retries); the
// eventual write of the result is itself guarded by the match's status, so a result is applied at most
// once even if RunMatch is called again after it already finished.
func (s *Service) RunMatch(ctx context.Context, matchID string) error {
	var kind, mapName string
	var seed int64
	var ticks int
	var participants []playerInput
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `UPDATE matches SET status = 'running', started_at = now()
			WHERE id = $1 AND status IN ('queued', 'running') RETURNING kind, seed, map, ticks`, matchID).
			Scan(&kind, &seed, &mapName, &ticks)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT mp.bot_id, mp.version_id, gb.name, gb.house, bv.source, bv.number, bv.language, bv.entry, bv.archive
			FROM match_players mp
			JOIN game_bots gb ON gb.id = mp.bot_id
			JOIN bot_versions bv ON bv.id = mp.version_id
			WHERE mp.match_id = $1 ORDER BY mp.slot`, matchID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p playerInput
			if err := rows.Scan(&p.BotID, &p.VersionID, &p.Name, &p.House, &p.Source, &p.Number, &p.Language, &p.Entry, &p.Archive); err != nil {
				return err
			}
			participants = append(participants, p)
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already finished or infra_error; nothing to do
	}
	if err != nil {
		return err
	}

	players, cleanup, err := s.launchSpecs(participants)
	defer cleanup()
	if err != nil {
		return err
	}

	timeout := time.Duration(ticks)*200*time.Millisecond + 60*time.Second
	matchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	runStart := time.Now()
	result, err := match.Run(matchCtx, s.l, match.Config{Seed: seed, Map: mapName, Ticks: ticks}, players)
	metrics.MatchRunSeconds.Observe(time.Since(runStart).Seconds())
	if err != nil {
		return err
	}
	return s.finishMatch(ctx, matchID, participants, result)
}

// finishMatch writes a played match's result in one transaction, guarded by the match still being
// running: a race with a concurrent finish (or with SweepStuck marking it infra_error) leaves it alone.
func (s *Service) finishMatch(ctx context.Context, matchID string, participants []playerInput, result match.Result) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var kind string
		err := tx.QueryRow(ctx, `UPDATE matches SET status = 'finished', finished_at = now()
			WHERE id = $1 AND status = 'running' RETURNING kind`, matchID).Scan(&kind)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		metrics.MatchFinished("finished")

		var before, after []rating.Rating
		if kind == "ladder" {
			before, after, err = s.applyRatings(ctx, tx, participants, result)
			if err != nil {
				return err
			}
		}

		for i := range participants {
			o := result.Players[i]
			stderr := sanitize.CleanLog(o.Stderr, 16<<10)
			if before != nil {
				if _, err := tx.Exec(ctx, `UPDATE match_players SET place = $3, kills = $4, damage = $5, death_tick = $6,
					status = $7, answered = $8, asked = $9, noise = $10, stderr_tail = $11,
					mu_before = $12, sigma_before = $13, mu_after = $14, sigma_after = $15
					WHERE match_id = $1 AND slot = $2`,
					matchID, i, o.Place, o.Kills, o.Damage, o.DeathTick, o.Status, o.Answered, o.Asked, o.Noise, stderr,
					before[i].Mu, before[i].Sigma, after[i].Mu, after[i].Sigma); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE match_players SET place = $3, kills = $4, damage = $5, death_tick = $6,
				status = $7, answered = $8, asked = $9, noise = $10, stderr_tail = $11
				WHERE match_id = $1 AND slot = $2`,
				matchID, i, o.Place, o.Kills, o.Damage, o.DeathTick, o.Status, o.Answered, o.Asked, o.Noise, stderr); err != nil {
				return err
			}
		}

		return insertReplay(ctx, tx, matchID, participants, result)
	})
}

// applyRatings locks the match's bots (FOR UPDATE, ordered by id to avoid deadlocking against a
// concurrent match sharing a bot), applies rating.Update from their current mu/sigma and places, and
// writes each bot's new rating, matches and wins back. It returns each bot's rating before and after,
// aligned with participants, for the caller to record on the match_players rows.
func (s *Service) applyRatings(ctx context.Context, tx pgx.Tx, participants []playerInput, result match.Result) ([]rating.Rating, []rating.Rating, error) {
	ids := make([]string, len(participants))
	for i, p := range participants {
		ids[i] = p.BotID
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)

	rows, err := tx.Query(ctx, `SELECT id, mu, sigma FROM game_bots WHERE id = ANY($1) ORDER BY id FOR UPDATE`, sorted)
	if err != nil {
		return nil, nil, err
	}
	current := make(map[string]rating.Rating, len(ids))
	for rows.Next() {
		var id string
		var r rating.Rating
		if err := rows.Scan(&id, &r.Mu, &r.Sigma); err != nil {
			rows.Close()
			return nil, nil, err
		}
		current[id] = r
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	rows.Close()

	before := make([]rating.Rating, len(participants))
	places := make([]int, len(participants))
	for i, p := range participants {
		before[i] = current[p.BotID]
		places[i] = result.Players[i].Place
	}
	after := rating.Update(before, places)

	for i, p := range participants {
		win := 0
		if places[i] == 1 {
			win = 1
		}
		if _, err := tx.Exec(ctx, `UPDATE game_bots SET mu = $2, sigma = $3, matches = matches + 1, wins = wins + $4, last_match_at = now()
			WHERE id = $1`, p.BotID, after[i].Mu, after[i].Sigma, win); err != nil {
			return nil, nil, err
		}
	}
	return before, after, nil
}

// leaderboardAll loads every active bot (except the idle house bot, which exists only for qualification
// checks) and ranks them by their displayed rating, highest first.
func (s *Service) leaderboardAll(ctx context.Context) ([]LeaderboardEntry, error) {
	var out []LeaderboardEntry
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT g.id, g.name, g.mu, g.sigma, g.matches, g.wins, g.house, v.source, v.number
			FROM game_bots g JOIN bot_versions v ON v.id = g.active_version_id
			WHERE g.active_version_id IS NOT NULL AND g.id <> 'bot_house_idle'`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e LeaderboardEntry
			if err := rows.Scan(&e.BotID, &e.Name, &e.Mu, &e.Sigma, &e.Matches, &e.Wins, &e.House, &e.Source, &e.Version); err != nil {
				return err
			}
			e.Rating = rating.Display(rating.Rating{Mu: e.Mu, Sigma: e.Sigma})
			out = append(out, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rating > out[j].Rating })
	for i := range out {
		out[i].Rank = i + 1
	}
	return out, nil
}

// Leaderboard returns the top limit active bots by rating (or all of them, if limit <= 0 or larger than
// the leaderboard).
func (s *Service) Leaderboard(ctx context.Context, limit int) ([]LeaderboardEntry, error) {
	all, err := s.leaderboardAll(ctx)
	if err != nil {
		return nil, err
	}
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	if all == nil {
		all = []LeaderboardEntry{}
	}
	return all, nil
}

// Bot returns one bot's public profile: its leaderboard row (Rank 0 if it isn't listed there) and its
// full version history, newest first.
func (s *Service) Bot(ctx context.Context, id string) (BotProfile, error) {
	var out BotProfile
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var mu, sigma float64
		var matches, wins int
		var name string
		var house bool
		var createdAt time.Time
		var activeVersionID *string
		if err := tx.QueryRow(ctx, `SELECT name, house, mu, sigma, matches, wins, created_at, active_version_id
			FROM game_bots WHERE id = $1`, id).
			Scan(&name, &house, &mu, &sigma, &matches, &wins, &createdAt, &activeVersionID); err != nil {
			return err
		}
		out.LeaderboardEntry = LeaderboardEntry{
			BotID: id, Name: name, House: house, Mu: mu, Sigma: sigma, Matches: matches, Wins: wins,
			Rating: rating.Display(rating.Rating{Mu: mu, Sigma: sigma}),
		}
		out.CreatedAt = createdAt.UTC()

		if activeVersionID != nil {
			if err := tx.QueryRow(ctx, `SELECT source, number FROM bot_versions WHERE id = $1`, *activeVersionID).
				Scan(&out.Source, &out.Version); err != nil {
				return err
			}
			if id != "bot_house_idle" {
				var rank int
				if err := tx.QueryRow(ctx, `
					SELECT count(*) + 1 FROM game_bots g2
					WHERE g2.active_version_id IS NOT NULL AND g2.id <> 'bot_house_idle' AND g2.id <> $1
					  AND (1000 + 40 * (g2.mu - 3 * g2.sigma)) > (1000 + 40 * ($2 - 3 * $3))`, id, mu, sigma).Scan(&rank); err != nil {
					return err
				}
				out.Rank = rank
			}
		}

		rows, err := tx.Query(ctx, `SELECT number, source, status, created_at FROM bot_versions WHERE bot_id = $1 ORDER BY number DESC`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var vv VersionPublic
			if err := rows.Scan(&vv.Number, &vv.Source, &vv.Status, &vv.CreatedAt); err != nil {
				return err
			}
			vv.CreatedAt = vv.CreatedAt.UTC()
			out.Versions = append(out.Versions, vv)
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return BotProfile{}, httpx.NotFound()
	}
	if err != nil {
		return BotProfile{}, err
	}
	if out.Versions == nil {
		out.Versions = []VersionPublic{}
	}
	return out, nil
}

// matchViewTx loads one match with its players.
func (s *Service) matchViewTx(ctx context.Context, tx pgx.Tx, id string) (MatchView, error) {
	var mv MatchView
	err := tx.QueryRow(ctx, `
		SELECT m.id, m.kind, m.status, m.map, m.seed, m.ticks, m.featured, m.created_at, m.started_at, m.finished_at,
		       EXISTS (SELECT 1 FROM match_replays r WHERE r.match_id = m.id)
		FROM matches m WHERE m.id = $1`, id).
		Scan(&mv.ID, &mv.Kind, &mv.Status, &mv.Map, &mv.Seed, &mv.Ticks, &mv.Featured, &mv.CreatedAt, &mv.StartedAt, &mv.FinishedAt, &mv.HasReplay)
	if err != nil {
		return MatchView{}, err
	}
	mv.CreatedAt = mv.CreatedAt.UTC()
	if mv.StartedAt != nil {
		u := mv.StartedAt.UTC()
		mv.StartedAt = &u
	}
	if mv.FinishedAt != nil {
		u := mv.FinishedAt.UTC()
		mv.FinishedAt = &u
	}

	rows, err := tx.Query(ctx, `
		SELECT mp.slot, mp.bot_id, gb.name, gb.house, bv.source, bv.number,
		       mp.place, mp.kills, mp.damage, mp.death_tick, mp.status,
		       mp.mu_before, mp.sigma_before, mp.mu_after, mp.sigma_after
		FROM match_players mp
		JOIN game_bots gb ON gb.id = mp.bot_id
		JOIN bot_versions bv ON bv.id = mp.version_id
		WHERE mp.match_id = $1 ORDER BY mp.slot`, id)
	if err != nil {
		return MatchView{}, err
	}
	defer rows.Close()
	mv.Players = []MatchPlayerView{}
	for rows.Next() {
		var pv MatchPlayerView
		var muBefore, sigmaBefore, muAfter, sigmaAfter *float64
		if err := rows.Scan(&pv.Slot, &pv.BotID, &pv.Name, &pv.House, &pv.Source, &pv.Version,
			&pv.Place, &pv.Kills, &pv.Damage, &pv.DeathTick, &pv.Status,
			&muBefore, &sigmaBefore, &muAfter, &sigmaAfter); err != nil {
			return MatchView{}, err
		}
		if muBefore != nil && sigmaBefore != nil {
			d := rating.Display(rating.Rating{Mu: *muBefore, Sigma: *sigmaBefore})
			pv.RatingBefore = &d
		}
		if muAfter != nil && sigmaAfter != nil {
			d := rating.Display(rating.Rating{Mu: *muAfter, Sigma: *sigmaAfter})
			pv.RatingAfter = &d
		}
		mv.Players = append(mv.Players, pv)
	}
	return mv, rows.Err()
}

// Match returns one match with its players; 404 not_found if it doesn't exist.
func (s *Service) Match(ctx context.Context, id string) (MatchView, error) {
	var mv MatchView
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		mv, err = s.matchViewTx(ctx, tx, id)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return MatchView{}, httpx.NotFound()
	}
	if err != nil {
		return MatchView{}, err
	}
	return mv, nil
}

func (s *Service) finishedLadderMatchIDs(ctx context.Context, tx pgx.Tx, botID string, limit int) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT m.id FROM matches m JOIN match_players mp ON mp.match_id = m.id
		WHERE mp.bot_id = $1 AND m.kind = 'ladder' AND m.status = 'finished'
		ORDER BY m.finished_at DESC LIMIT $2`, botID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// matchesTx is Matches, inside the caller's transaction - used by MyTanks to fold a bot's recent matches
// into the same read as the rest of its page.
func (s *Service) matchesTx(ctx context.Context, tx pgx.Tx, botID string, limit int) ([]MatchView, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	ids, err := s.finishedLadderMatchIDs(ctx, tx, botID, limit)
	if err != nil {
		return nil, err
	}
	out := []MatchView{}
	for _, id := range ids {
		mv, err := s.matchViewTx(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, mv)
	}
	return out, nil
}

// Matches returns botID's finished ladder matches, newest first, up to limit (capped at 50).
func (s *Service) Matches(ctx context.Context, botID string, limit int) ([]MatchView, error) {
	var out []MatchView
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = s.matchesTx(ctx, tx, botID, limit)
		return err
	})
	return out, err
}

// Replay returns a match's gzip-compressed replay; 404 not_found if it has none (never played, pruned, or
// unknown match id).
func (s *Service) Replay(ctx context.Context, id string) ([]byte, error) {
	var data []byte
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT data FROM match_replays WHERE match_id = $1`, id).Scan(&data)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.NotFound()
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// MatchLog returns the stderr tail for userID's own bot's slot in matchID; 404 not_found if userID has no
// bot in that match (including an unknown match id).
func (s *Service) MatchLog(ctx context.Context, userID, matchID string) (MatchLog, error) {
	var out MatchLog
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT mp.slot, mp.stderr_tail FROM match_players mp
			JOIN game_bots gb ON gb.id = mp.bot_id
			WHERE mp.match_id = $1 AND gb.owner_user_id = $2`, matchID, userID).
			Scan(&out.Slot, &out.Stderr)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return MatchLog{}, httpx.NotFound()
	}
	if err != nil {
		return MatchLog{}, err
	}
	out.MatchID = matchID
	return out, nil
}

// markMatchPlatformFailure is called once a run_match job has used every retry: the platform, not the
// bots, is at fault, so the match is marked infra_error (no rating change) rather than left stuck in
// queued or running forever.
func (s *Service) markMatchPlatformFailure(ctx context.Context, matchID string) error {
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE matches SET status = 'infra_error', failure_reason = 'platform', finished_at = now()
			WHERE id = $1 AND status IN ('queued', 'running')`, matchID)
		if err == nil && tag.RowsAffected() > 0 {
			metrics.MatchFinished("infra_error")
		}
		return err
	})
	return err
}

// SweepStuck moves every match that has been queued or running for more than 10 minutes to infra_error
// (no rating change - the platform failed it, the bots didn't) and returns how many it swept.
func (s *Service) SweepStuck(ctx context.Context) (int, error) {
	var n int64
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE matches SET status = 'infra_error', failure_reason = 'stuck', finished_at = now()
			WHERE status IN ('queued', 'running') AND coalesce(started_at, created_at) < now() - interval '10 minutes'`)
		n = tag.RowsAffected()
		return err
	})
	if err == nil && n > 0 {
		metrics.MatchesFinishedAdd("infra_error", int(n))
	}
	return int(n), err
}

// PruneReplays deletes replays for matches finished more than 3 days ago, except featured matches and
// check matches, and returns how many it deleted.
func (s *Service) PruneReplays(ctx context.Context) (int, error) {
	var n int64
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM match_replays r USING matches m
			WHERE r.match_id = m.id AND m.kind <> 'check' AND m.featured = false
			  AND m.finished_at IS NOT NULL AND m.finished_at < now() - interval '3 days'`)
		n = tag.RowsAffected()
		return err
	})
	return int(n), err
}
