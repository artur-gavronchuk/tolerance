package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/rating"
	"tolerance/internal/games/tanks"
)

type seedBot struct {
	id, name, owner string
	house           bool
	versionID       string
	versionNo       int
	source          string
	strength        float64 // true strength, drives results
	mu, sigma       float64 // current season
	matches, wins   int
	lifeMu, lifeSig float64
	created         time.Time
}

var starterBots = []string{"ace", "duelist", "hunter", "sniper", "warden"}

func (s *seeder) tanks(ctx context.Context, tx pgx.Tx) error {
	var bots []*seedBot
	var botRows, verRows [][]any
	active := map[string]string{}
	for ui := range s.usersList {
		u := &s.usersList[ui]
		if ui >= 3 && s.rng.Float64() > 0.2+0.6*u.activity { // ~40% of people have a tank (the first few always do)
			continue
		}
		b := &seedBot{
			id: fmt.Sprintf("%sb%04d", idPrefix, ui+1), name: strings.ReplaceAll(strings.Title(strings.ReplaceAll(u.handle, "-", " ")), " ", ""),
			owner: u.id, strength: 0.6*u.skill + 0.4*s.rng.Float64(), created: u.created.Add(time.Duration(s.rng.Intn(5*24)) * time.Hour),
			source: "upload",
		}
		if b.created.After(s.now) {
			b.created = s.now.Add(-time.Hour)
		}
		u.botID = b.id
		b.versionNo = 1 + s.rng.Intn(4)
		b.versionID = fmt.Sprintf("%sbv%04d_%d", idPrefix, ui+1, b.versionNo)
		b.matches = 8 + s.rng.Intn(60)
		b.sigma = math.Max(1.6, rating.DefaultSigma-0.12*float64(b.matches))
		b.mu = 25 + 14*(b.strength-0.45) + 0.8*s.rng.NormFloat64()
		b.wins = int(math.Round(float64(b.matches) * math.Min(0.9, math.Max(0.05, b.strength*0.8+0.05*s.rng.NormFloat64()))))
		b.lifeMu, b.lifeSig = b.mu+0.5*s.rng.NormFloat64(), b.sigma
		bots = append(bots, b)
		botRows = append(botRows, []any{b.id, "tanks", b.owner, b.name, false, b.lifeMu, b.lifeSig, b.matches + 20, b.wins + 8, b.created})
		for n := 1; n <= b.versionNo; n++ {
			entry := starterBots[s.rng.Intn(len(starterBots))]
			vid := fmt.Sprintf("%sbv%04d_%d", idPrefix, ui+1, n)
			verRows = append(verRows, []any{vid, b.id, n, "upload", "builtin", entry, "active", b.created.Add(time.Duration(n) * 6 * time.Hour)})
		}
		active[b.id] = b.versionID
	}
	s.counts["bots"] = len(bots)
	if err := s.copy(ctx, tx, "game_bots", []string{"id", "game", "owner_user_id", "name", "house", "mu", "sigma", "matches", "wins", "created_at"}, botRows); err != nil {
		return err
	}
	if err := s.copy(ctx, tx, "bot_versions", []string{"id", "bot_id", "number", "source", "language", "entry", "status", "created_at"}, verRows); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE game_bots g SET active_version_id = 'seed_bv' || substr(g.id, 7) || '_' || (SELECT max(number) FROM bot_versions v WHERE v.bot_id = g.id)
		WHERE g.owner_user_id IN `+seedUsersSQL); err != nil {
		return err
	}

	// House bots take part too: fixed strengths, weakest first.
	for i, name := range []string{"duelist", "hunter", "warden", "sniper", "ace"} {
		st := 0.15 + 0.17*float64(i)
		b := &seedBot{id: "bot_house_" + name, name: name, house: true, versionID: "bv_house_" + name, versionNo: 1, source: "house", strength: st,
			matches: 120, sigma: 1.6, mu: 25 + 14*(st-0.45), created: s.now.AddDate(0, 0, -60)}
		b.wins = int(float64(b.matches) * st)
		bots = append(bots, b)
		if _, err := tx.Exec(ctx, `UPDATE game_bots SET mu = $2, sigma = $3, matches = $4, wins = $5 WHERE id = $1`, b.id, b.mu, b.sigma, b.matches, b.wins); err != nil {
			return err
		}
	}

	if err := s.seasons(ctx, tx, bots); err != nil {
		return err
	}
	matchIDs, err := s.ladderMatches(ctx, tx, bots)
	if err != nil {
		return err
	}
	return s.tournaments(ctx, tx, bots, matchIDs)
}

func seasonID(t time.Time) (string, time.Time, time.Time) {
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start.Format("2006-01"), start, start.AddDate(0, 1, 0)
}

func (s *seeder) seasons(ctx context.Context, tx pgx.Tx, bots []*seedBot) error {
	curID, curStart, curEnd := seasonID(s.now)
	if _, err := tx.Exec(ctx, `INSERT INTO tanks_seasons (id, starts_at, ends_at) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`, curID, curStart, curEnd); err != nil {
		return err
	}
	var rows [][]any
	for _, b := range bots {
		if b.house {
			if _, err := tx.Exec(ctx, `INSERT INTO tanks_season_ratings (season_id, bot_id, mu, sigma, matches, wins) VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (season_id, bot_id) DO UPDATE SET mu = $3, sigma = $4, matches = $5, wins = $6`, curID, b.id, b.mu, b.sigma, 30, 30*int(b.strength*100)/100); err != nil {
				return err
			}
			continue
		}
		m := int(float64(b.matches) * 0.3)
		rows = append(rows, []any{curID, b.id, b.mu, math.Max(2, b.sigma+1), m, int(float64(m) * b.strength)})
	}
	if err := s.copy(ctx, tx, "tanks_season_ratings", []string{"season_id", "bot_id", "mu", "sigma", "matches", "wins"}, rows); err != nil {
		return err
	}

	// Two archived seasons with frozen standings.
	for back := 1; back <= 2; back++ {
		id, start, end := seasonID(curStart.AddDate(0, -back, 0))
		if _, err := tx.Exec(ctx, `INSERT INTO tanks_seasons (id, starts_at, ends_at, status, finalized_at) VALUES ($1, $2, $3, 'archived', $3) ON CONFLICT (id) DO NOTHING`, id, start, end); err != nil {
			return err
		}
		type st struct {
			b      *seedBot
			mu, sg float64
			m, w   int
		}
		var list []st
		for _, b := range bots {
			if s.rng.Float64() > 0.95-0.25*float64(back-1) {
				continue
			}
			mu := b.mu + 2*s.rng.NormFloat64()
			sg := math.Max(1.7, 5+2*s.rng.Float64())
			m := 10 + s.rng.Intn(90)
			list = append(list, st{b, mu, sg, m, int(float64(m) * math.Min(0.9, b.strength*0.85))})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].mu-3*list[i].sg > list[j].mu-3*list[j].sg })
		var srows [][]any
		for i, e := range list {
			owner := ""
			if !e.b.house {
				owner = strings.TrimPrefix(e.b.id, idPrefix+"b")
				owner = s.usersList[atoiDefault(owner)-1].handle
			}
			srows = append(srows, []any{id, e.b.id, i + 1, rating.Display(rating.Rating{Mu: e.mu, Sigma: e.sg}), e.mu, e.sg, e.m, e.w, e.b.name, owner, e.b.house, e.b.source, e.b.versionNo})
		}
		if err := s.copy(ctx, tx, "tanks_season_standings", []string{"season_id", "bot_id", "rank", "rating", "mu", "sigma", "matches", "wins", "bot_name", "owner", "house", "source", "version"}, srows); err != nil {
			return err
		}
	}
	return nil
}

func atoiDefault(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// place orders bots by strength plus luck, best first.
func (s *seeder) place(players []*seedBot) []int {
	idx := make([]int, len(players))
	score := make([]float64, len(players))
	for i, p := range players {
		idx[i] = i
		score[i] = p.strength + 0.25*s.rng.NormFloat64()
	}
	sort.Slice(idx, func(a, b int) bool { return score[idx[a]] > score[idx[b]] })
	places := make([]int, len(players))
	for rank, i := range idx {
		places[i] = rank + 1
	}
	return places
}

type matchSpec struct {
	id, kind string
	players  []*seedBot
	places   []int
	at       time.Time
}

func (s *seeder) insertMatches(ctx context.Context, tx pgx.Tx, ms []matchSpec) error {
	maps := tanks.Maps()
	ticks := tanks.DefaultRules().Ticks
	var mrows, prows [][]any
	for i, m := range ms {
		seed := s.rng.Int63n(1 << 53)
		mp := maps[int(seed%int64(len(maps)))]
		mrows = append(mrows, []any{m.id, "tanks", m.kind, "finished", seed, mp.Name, ticks - s.rng.Intn(ticks/3), i%40 == 0, "", m.at.Add(-2 * time.Minute), m.at.Add(-time.Minute), m.at})
		for slot, p := range m.players {
			place := m.places[slot]
			kills := 0
			if place == 1 {
				kills = 1 + s.rng.Intn(3)
			} else if place == 2 {
				kills = s.rng.Intn(2)
			}
			var death any
			if place > 1 {
				death = 200 + s.rng.Intn(ticks-200)
			}
			delta := 1.5 * float64(3-place) / 2
			prows = append(prows, []any{m.id, slot, p.id, p.versionID, place, kills, 20 + s.rng.Intn(180) + 40*(4-place), death, "ok", 90 + s.rng.Intn(10), 100, 0,
				p.mu - delta, p.sigma + 0.1, p.mu, p.sigma})
		}
	}
	s.counts["matches"] += len(ms)
	if err := s.copy(ctx, tx, "matches", []string{"id", "game", "kind", "status", "seed", "map", "ticks", "featured", "failure_reason", "created_at", "started_at", "finished_at"}, mrows); err != nil {
		return err
	}
	return s.copy(ctx, tx, "match_players", []string{"match_id", "slot", "bot_id", "version_id", "place", "kills", "damage", "death_tick", "status", "answered", "asked", "noise",
		"mu_before", "sigma_before", "mu_after", "sigma_after"}, prows)
}

func (s *seeder) ladderMatches(ctx context.Context, tx pgx.Tx, bots []*seedBot) ([]string, error) {
	var ms []matchSpec
	var ids []string
	for i := 0; i < s.n*14/10; i++ {
		var players []*seedBot
		seen := map[string]bool{}
		for len(players) < 4 {
			b := bots[s.rng.Intn(len(bots))]
			if seen[b.id] {
				continue
			}
			seen[b.id] = true
			players = append(players, b)
		}
		at := s.now.Add(-time.Duration(s.rng.Intn(30*24*60)) * time.Minute)
		id := fmt.Sprintf("%sm%d", idPrefix, i)
		ids = append(ids, id)
		ms = append(ms, matchSpec{id, "ladder", players, s.place(players), at})
	}
	// A recent broadcast schedule row so the live page has something to show.
	if err := s.insertMatches(ctx, tx, ms); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *seeder) tournaments(ctx context.Context, tx pgx.Tx, bots []*seedBot, _ []string) error {
	cur, curStart, _ := seasonID(s.now)
	_ = curStart
	var real []*seedBot
	for _, b := range bots {
		real = append(real, b)
	}
	sort.Slice(real, func(i, j int) bool { return real[i].strength > real[j].strength })
	seedOrder := [][2]int{{1, 8}, {4, 5}, {2, 7}, {3, 6}} // standard bracket: pairs of seeds in round 1
	for t := 0; t < 4; t++ {
		starts := s.today.AddDate(0, 0, -3-7*t).Add(18 * time.Hour)
		if starts.After(s.now) {
			starts = starts.AddDate(0, 0, -7)
		}
		sid, _, _ := seasonID(starts)
		if sid != cur {
			// Past-season tournaments still need their season row; the archived ones were created above.
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tanks_seasons WHERE id = $1)`, sid).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				sid = cur
			}
		}
		tid := fmt.Sprintf("%st%d", idPrefix, t)
		// A different 8 each week: the top 12 shuffled in by luck.
		pool := append([]*seedBot(nil), real[:12]...)
		sort.Slice(pool, func(i, j int) bool {
			return pool[i].strength+0.15*s.rng.NormFloat64() > pool[j].strength+0.15*s.rng.NormFloat64()
		})
		top := pool[:8]
		if _, err := tx.Exec(ctx, `INSERT INTO tanks_tournaments (id, name, season_id, status, starts_at, started_at, finished_at, size, rounds, best_of)
			VALUES ($1, $2, $3, 'finished', $4, $4, $5, 8, 3, 3)`, tid, "Weekly tournament, "+starts.Format("2 Jan"), sid, starts, starts.Add(40*time.Minute)); err != nil {
			return err
		}
		for i, b := range top {
			if _, err := tx.Exec(ctx, `INSERT INTO tanks_tournament_entries (tournament_id, bot_id, version_id, seed, rating) VALUES ($1, $2, $3, $4, $5)`,
				tid, b.id, b.versionID, i+1, rating.Display(rating.Rating{Mu: b.mu, Sigma: b.sigma})); err != nil {
				return err
			}
		}
		// Round 1 as in a seeded bracket, then winners meet.
		cur := make([]*seedBot, 0, 8)
		for _, p := range seedOrder {
			cur = append(cur, top[p[0]-1], top[p[1]-1])
		}
		var champ *seedBot
		var ms []matchSpec
		var games [][]any
		for round := 1; round <= 3; round++ {
			var next []*seedBot
			for pos := 0; pos < len(cur)/2; pos++ {
				a, b := cur[2*pos], cur[2*pos+1]
				wa, wb := 0, 0
				pid := fmt.Sprintf("%stp%d_%d_%d", idPrefix, t, round, pos)
				for g := 1; wa < 2 && wb < 2; g++ {
					players := []*seedBot{a, b}
					places := s.place(players)
					mid := fmt.Sprintf("%stm%d_%d_%d_%d", idPrefix, t, round, pos, g)
					ms = append(ms, matchSpec{mid, "tournament", players, places, starts.Add(time.Duration(round*10+g) * time.Minute)})
					var win *seedBot
					if places[0] == 1 {
						wa++
						win = a
					} else {
						wb++
						win = b
					}
					games = append(games, []any{pid, g, mid, win.id})
				}
				winner := a
				if wb > wa {
					winner = b
				}
				if _, err := tx.Exec(ctx, `INSERT INTO tanks_tournament_pairings (id, tournament_id, round, position, bot_a, bot_b, wins_a, wins_b, status, winner_bot_id)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'finished', $9)`, pid, tid, round, pos, a.id, b.id, wa, wb, winner.id); err != nil {
					return err
				}
				next = append(next, winner)
			}
			cur = next
			champ = cur[0]
		}
		if err := s.insertMatches(ctx, tx, ms); err != nil {
			return err
		}
		if err := s.copy(ctx, tx, "tanks_tournament_games", []string{"pairing_id", "game", "match_id", "winner_bot_id"}, games); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tanks_tournaments SET champion_bot_id = $2 WHERE id = $1`, tid, champ.id); err != nil {
			return err
		}
		s.counts["tournaments"]++
	}
	return nil
}
