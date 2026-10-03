package games

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/tanks"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/jobs"
)

// A tournament is a single-elimination bracket among the top bots of the current season ladder. Every
// pairing is a best-of-3 series of 1v1 matches on different maps, run through the ordinary match queue
// (kind 'tournament': the match runner and its replays are the ladder's, but no ratings change). The games
// worker advances the bracket (TournamentTick) as the matches finish.
const (
	// DefaultTournamentSize is how many of the season's top bots enter a weekly tournament.
	DefaultTournamentSize = 8
	tournamentBestOf      = 3
	// The weekly tournament starts every tournamentWeekday at tournamentHour:00 UTC.
	tournamentWeekday = time.Saturday
	tournamentHour    = 18
	// A pairing whose matches fail on the platform this many times is decided by seed.
	maxPairingInfraErrors = 3
	// On-demand tournaments are named with this prefix; it is how they are told apart from the weekly one.
	openTournamentPrefix = "Open tournament"
)

// TournamentBot is one entrant.
type TournamentBot struct {
	BotID string `json:"bot_id"`
	Name  string `json:"name"`
	House bool   `json:"house"`
	Seed  int    `json:"seed"`
	Owner string `json:"owner"`
}

// TournamentGame is one match of a series.
type TournamentGame struct {
	Game        int     `json:"game"`
	MatchID     string  `json:"match_id"`
	Status      string  `json:"status"`
	Map         string  `json:"map"`
	WinnerBotID *string `json:"winner_bot_id"`
	HasReplay   bool    `json:"has_replay"`
}

// Pairing is one bracket slot: two bots (either may still be unknown), the series score and its matches.
type Pairing struct {
	ID          string           `json:"id"`
	Round       int              `json:"round"`
	Position    int              `json:"position"`
	A           *TournamentBot   `json:"a"`
	B           *TournamentBot   `json:"b"`
	WinsA       int              `json:"wins_a"`
	WinsB       int              `json:"wins_b"`
	Status      string           `json:"status"` // pending | running | finished
	WinnerBotID *string          `json:"winner_bot_id"`
	Bye         bool             `json:"bye"`
	Games       []TournamentGame `json:"games"`
}

// TournamentView is a tournament; Entries and Pairings are only filled in the detail view.
type TournamentView struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	SeasonID   *string         `json:"season_id"`
	Status     string          `json:"status"` // scheduled | running | finished | cancelled
	StartsAt   time.Time       `json:"starts_at"`
	StartedAt  *time.Time      `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at"`
	Size       int             `json:"size"`
	Rounds     int             `json:"rounds"`
	BestOf     int             `json:"best_of"`
	EntryCount int             `json:"entry_count"`
	Champion   *TournamentBot  `json:"champion"`
	Open       bool            `json:"open"` // started on demand (any 2+ bots), not part of the weekly schedule
	Entries    []TournamentBot `json:"entries"`
	Pairings   []Pairing       `json:"pairings"`
	Now        time.Time       `json:"now"`
}

// BotTournament is one tournament a bot took part in, for its profile.
type BotTournament struct {
	TournamentID string    `json:"tournament_id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	StartsAt     time.Time `json:"starts_at"`
	Seed         int       `json:"seed"`
	Rounds       int       `json:"rounds"`
	Result       string    `json:"result"`
	Champion     bool      `json:"champion"`
	Open         bool      `json:"open"` // started on demand, not the weekly one
}

// Showcase is everything the /tanks front page needs in one call.
type Showcase struct {
	Now             time.Time          `json:"now"`
	Season          SeasonView         `json:"season"`
	Ladder          []LeaderboardEntry `json:"ladder"`
	Tournament      *TournamentView    `json:"tournament"`       // running, or finished within the last day: with its bracket
	NextTournament  *TournamentView    `json:"next_tournament"`  // the next scheduled weekly one
	OpenTournaments []TournamentView   `json:"open_tournaments"` // on-demand tournaments running now or finished within the last day
	Champions       []TournamentView   `json:"champions"`        // latest finished tournaments, newest first
	Notable         []MatchView        `json:"notable"`
	PastSeasons     []SeasonView       `json:"past_seasons"`
}

// nextTournamentSlot is the first weekly start strictly after t.
func nextTournamentSlot(t time.Time) time.Time {
	t = t.UTC()
	c := time.Date(t.Year(), t.Month(), t.Day(), tournamentHour, 0, 0, 0, time.UTC)
	for c.Weekday() != tournamentWeekday || !c.After(t) {
		c = c.AddDate(0, 0, 1)
	}
	return c
}

// seedOrder lists the seeds of a bracket of size n (a power of two) in pairing order: pairing p is
// seeds order[2p] vs order[2p+1], and winners of pairings 2k and 2k+1 meet in the next round, so seeds 1
// and 2 can only meet in the final.
func seedOrder(n int) []int {
	order := []int{1}
	for len(order) < n {
		next := make([]int, 0, len(order)*2)
		total := len(order)*2 + 1
		for _, s := range order {
			next = append(next, s, total-s)
		}
		order = next
	}
	return order
}

// TournamentTick drives tournaments: it keeps one weekly tournament scheduled, starts the ones that are
// due, and advances every running bracket (records finished matches, queues the next ones). Safe to call
// from several processes.
func (s *Service) TournamentTick(ctx context.Context) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('games:tournaments'))`); err != nil {
			return err
		}

		var scheduled int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM tanks_tournaments WHERE status = 'scheduled'`).Scan(&scheduled); err != nil {
			return err
		}
		if scheduled == 0 {
			at := nextTournamentSlot(time.Now())
			if _, err := tx.Exec(ctx, `INSERT INTO tanks_tournaments (id, name, status, starts_at, size, best_of)
				VALUES ($1, $2, 'scheduled', $3, $4, $5)`,
				idgen.New("tourn"), "Weekly tournament, "+at.Format("2 Jan"), at, DefaultTournamentSize, tournamentBestOf); err != nil {
				return err
			}
		}

		rows, err := tx.Query(ctx, `SELECT id, size FROM tanks_tournaments WHERE status = 'scheduled' AND starts_at <= now()`)
		if err != nil {
			return err
		}
		type due struct {
			id   string
			size int
		}
		var dues []due
		for rows.Next() {
			var d due
			if err := rows.Scan(&d.id, &d.size); err != nil {
				rows.Close()
				return err
			}
			dues = append(dues, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, d := range dues {
			ok, err := s.startTournamentTx(ctx, tx, d.id, d.size)
			if err != nil {
				return err
			}
			if !ok {
				s.log.Info("games: tournament cancelled, not enough ranked bots", "tournament", d.id)
			}
		}

		rows, err = tx.Query(ctx, `SELECT id FROM tanks_tournaments WHERE status = 'running' ORDER BY started_at`)
		if err != nil {
			return err
		}
		var running []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			running = append(running, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range running {
			if err := s.advanceTournamentTx(ctx, tx, id); err != nil {
				return fmt.Errorf("advance tournament %s: %w", id, err)
			}
		}
		return nil
	})
}

// StartTournament starts an extra tournament right now among the top size bots of the current season
// (admin / local demo). size must be 2, 4, 8 or 16; 0 means the default.
func (s *Service) StartTournament(ctx context.Context, size int) (TournamentView, error) {
	if size == 0 {
		size = DefaultTournamentSize
	}
	if size != 2 && size != 4 && size != 8 && size != 16 {
		return TournamentView{}, httpx.WithField(422, "validation_failed", "size must be 2, 4, 8 or 16", "size", "invalid")
	}
	var id string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('games:tournaments'))`); err != nil {
			return err
		}
		now := time.Now().UTC()
		id = idgen.New("tourn")
		if _, err := tx.Exec(ctx, `INSERT INTO tanks_tournaments (id, name, status, starts_at, size, best_of)
			VALUES ($1, $2, 'scheduled', $3, $4, $5)`,
			id, openTournamentPrefix+", "+now.Format("2 Jan 15:04")+" UTC", now, size, tournamentBestOf); err != nil {
			return err
		}
		ok, err := s.startTournamentTx(ctx, tx, id, size)
		if err != nil {
			return err
		}
		if !ok {
			return httpx.New(422, "not_enough_bots", "At least two ranked bots are needed to start a tournament")
		}
		return nil
	})
	if err != nil {
		return TournamentView{}, err
	}
	return s.Tournament(ctx, id)
}

// preferSettled orders a ladder for tournament selection: bots with a settled (non-provisional) rating come
// first, provisional ones only fill the bracket when there are not enough of the others. The chosen bots stay
// in ladder order, so seeds still follow rating.
func preferSettled(ladder []LeaderboardEntry, size int) []LeaderboardEntry {
	var settled []LeaderboardEntry
	for _, e := range ladder {
		if !e.Provisional {
			settled = append(settled, e)
		}
	}
	if len(settled) >= size || len(settled) == len(ladder) {
		return settled
	}
	picked := map[string]bool{}
	for _, e := range settled {
		picked[e.BotID] = true
	}
	for _, e := range ladder {
		if len(picked) >= size {
			break
		}
		picked[e.BotID] = true
	}
	out := make([]LeaderboardEntry, 0, len(picked))
	for _, e := range ladder {
		if picked[e.BotID] {
			out = append(out, e)
		}
	}
	return out
}

// startTournamentTx seeds the bracket from the current season ladder and moves the tournament to running.
// It returns false (and cancels the tournament) when fewer than two bots are ranked.
func (s *Service) startTournamentTx(ctx context.Context, tx pgx.Tx, id string, size int) (bool, error) {
	season, err := ensureSeasonTx(ctx, tx)
	if err != nil {
		return false, err
	}
	ladder, err := seasonLadder(ctx, tx, season.ID, false)
	if err != nil {
		return false, err
	}
	ladder = preferSettled(ladder, size)
	n := min(size, len(ladder))
	if n < 2 {
		_, err := tx.Exec(ctx, `UPDATE tanks_tournaments SET status = 'cancelled' WHERE id = $1`, id)
		return false, err
	}
	top := ladder[:n]

	versions := map[string]string{}
	rows, err := tx.Query(ctx, `SELECT id, active_version_id FROM game_bots WHERE id = ANY($1)`, botIDs(top))
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var bot, ver string
		if err := rows.Scan(&bot, &ver); err != nil {
			rows.Close()
			return false, err
		}
		versions[bot] = ver
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, err
	}

	bracket, rounds := 2, 1
	for bracket < n {
		bracket *= 2
		rounds++
	}
	for i, e := range top {
		if _, err := tx.Exec(ctx, `INSERT INTO tanks_tournament_entries (tournament_id, bot_id, version_id, seed, rating)
			VALUES ($1, $2, $3, $4, $5)`, id, e.BotID, versions[e.BotID], i+1, e.Rating); err != nil {
			return false, err
		}
	}

	order := seedOrder(bracket)
	for r := 1; r <= rounds; r++ {
		for pos := 0; pos < bracket>>r; pos++ {
			var a, b *string
			bye := false
			status := "pending"
			var winner *string
			if r == 1 {
				sa, sb := order[2*pos], order[2*pos+1]
				if sa <= n {
					a = &top[sa-1].BotID
				}
				if sb <= n {
					b = &top[sb-1].BotID
				}
				if b == nil { // bye: the better seed goes straight through
					bye, status, winner = true, "finished", a
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO tanks_tournament_pairings
				(id, tournament_id, round, position, bot_a, bot_b, status, winner_bot_id, bye)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				idgen.New("tpair"), id, r, pos, a, b, status, winner, bye); err != nil {
				return false, err
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE tanks_tournaments SET status = 'running', started_at = now(), season_id = $2, rounds = $3
		WHERE id = $1`, id, season.ID, rounds); err != nil {
		return false, err
	}
	return true, s.advanceTournamentTx(ctx, tx, id)
}

func botIDs(es []LeaderboardEntry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.BotID
	}
	return out
}

// pairingState is a pairing row while the bracket is being advanced.
type pairingState struct {
	id           string
	round, pos   int
	botA, botB   *string
	winsA, winsB int
	status       string
	winner       *string
	infra        int
}

type bracketState struct {
	id       string
	rounds   int
	bestOf   int
	seeds    map[string]int
	versions map[string]string
	pairings map[[2]int]*pairingState
}

// advanceTournamentTx moves a running bracket as far as it can go without waiting for a match: it records
// finished matches, queues the next game of an open series, starts pairings whose two bots are known,
// carries winners into the next round and crowns the champion.
func (s *Service) advanceTournamentTx(ctx context.Context, tx pgx.Tx, id string) error {
	st := &bracketState{id: id, seeds: map[string]int{}, versions: map[string]string{}, pairings: map[[2]int]*pairingState{}}
	if err := tx.QueryRow(ctx, `SELECT rounds, best_of FROM tanks_tournaments WHERE id = $1 AND status = 'running'`, id).
		Scan(&st.rounds, &st.bestOf); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	rows, err := tx.Query(ctx, `SELECT bot_id, version_id, seed FROM tanks_tournament_entries WHERE tournament_id = $1`, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var bot, ver string
		var seed int
		if err := rows.Scan(&bot, &ver, &seed); err != nil {
			rows.Close()
			return err
		}
		st.versions[bot], st.seeds[bot] = ver, seed
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = tx.Query(ctx, `SELECT id, round, position, bot_a, bot_b, wins_a, wins_b, status, winner_bot_id, infra_errors
		FROM tanks_tournament_pairings WHERE tournament_id = $1`, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		p := &pairingState{}
		if err := rows.Scan(&p.id, &p.round, &p.pos, &p.botA, &p.botB, &p.winsA, &p.winsB, &p.status, &p.winner, &p.infra); err != nil {
			rows.Close()
			return err
		}
		st.pairings[[2]int{p.round, p.pos}] = p
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for iter := 0; iter < 32; iter++ {
		changed := false
		for r := 1; r <= st.rounds; r++ {
			for pos := 0; pos < 1<<(st.rounds-r); pos++ {
				p := st.pairings[[2]int{r, pos}]
				if p == nil {
					return fmt.Errorf("tournament %s: missing pairing %d/%d", id, r, pos)
				}
				switch {
				case p.status == "running":
					if err := s.stepPairing(ctx, tx, st, p); err != nil {
						return err
					}
				case p.status == "pending" && p.botA != nil && p.botB != nil:
					matchID, err := s.newTournamentMatch(ctx, tx, st, p, 1)
					if err != nil {
						return err
					}
					if _, err := tx.Exec(ctx, `INSERT INTO tanks_tournament_games (pairing_id, game, match_id) VALUES ($1, 1, $2)`, p.id, matchID); err != nil {
						return err
					}
					p.status = "running"
					if _, err := tx.Exec(ctx, `UPDATE tanks_tournament_pairings SET status = 'running' WHERE id = $1`, p.id); err != nil {
						return err
					}
					changed = true
				}
				if p.status == "finished" && p.winner != nil && r < st.rounds {
					next := st.pairings[[2]int{r + 1, pos / 2}]
					slot, col := &next.botA, "bot_a"
					if pos%2 == 1 {
						slot, col = &next.botB, "bot_b"
					}
					if *slot == nil {
						w := *p.winner
						*slot = &w
						if _, err := tx.Exec(ctx, `UPDATE tanks_tournament_pairings SET `+col+` = $2 WHERE id = $1`, next.id, w); err != nil {
							return err
						}
						changed = true
					}
				}
			}
		}
		if !changed {
			break
		}
	}

	if final := st.pairings[[2]int{st.rounds, 0}]; final.status == "finished" && final.winner != nil {
		if _, err := tx.Exec(ctx, `UPDATE tanks_tournaments SET status = 'finished', finished_at = now(), champion_bot_id = $2
			WHERE id = $1`, id, *final.winner); err != nil {
			return err
		}
		s.log.Info("games: tournament finished", "tournament", id, "champion", *final.winner)
	}
	return nil
}

// stepPairing looks at the latest match of a running series. A finished match is scored (and either ends
// the series or queues the next game); a platform failure replaces the match, or after too many decides
// the series by seed; a match still queued or running changes nothing.
func (s *Service) stepPairing(ctx context.Context, tx pgx.Tx, st *bracketState, p *pairingState) error {
	var game int
	var matchID, matchStatus string
	var gameWinner *string
	err := tx.QueryRow(ctx, `SELECT g.game, g.match_id, g.winner_bot_id, m.status
		FROM tanks_tournament_games g JOIN matches m ON m.id = g.match_id
		WHERE g.pairing_id = $1 ORDER BY g.game DESC LIMIT 1`, p.id).Scan(&game, &matchID, &gameWinner, &matchStatus)
	if errors.Is(err, pgx.ErrNoRows) { // a running series always has a game; recover rather than wedge
		mid, err := s.newTournamentMatch(ctx, tx, st, p, 1)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO tanks_tournament_games (pairing_id, game, match_id) VALUES ($1, 1, $2)`, p.id, mid)
		return err
	}
	if err != nil {
		return err
	}
	if gameWinner != nil {
		return nil // already scored; the series is decided below in the same transaction that scored it
	}

	switch matchStatus {
	case "queued", "running":
		return nil
	case "infra_error":
		p.infra++
		if _, err := tx.Exec(ctx, `UPDATE tanks_tournament_pairings SET infra_errors = $2 WHERE id = $1`, p.id, p.infra); err != nil {
			return err
		}
		if p.infra >= maxPairingInfraErrors {
			return s.finishPairing(ctx, tx, st, p, betterSeed(st, *p.botA, *p.botB))
		}
		mid, err := s.newTournamentMatch(ctx, tx, st, p, game)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE tanks_tournament_games SET match_id = $3 WHERE pairing_id = $1 AND game = $2`, p.id, game, mid)
		return err
	}

	// finished
	winner, err := s.matchWinner(ctx, tx, st, matchID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE tanks_tournament_games SET winner_bot_id = $3 WHERE pairing_id = $1 AND game = $2`, p.id, game, winner); err != nil {
		return err
	}
	if winner == *p.botA {
		p.winsA++
	} else {
		p.winsB++
	}
	if _, err := tx.Exec(ctx, `UPDATE tanks_tournament_pairings SET wins_a = $2, wins_b = $3 WHERE id = $1`, p.id, p.winsA, p.winsB); err != nil {
		return err
	}
	need := st.bestOf/2 + 1
	switch {
	case p.winsA >= need:
		return s.finishPairing(ctx, tx, st, p, *p.botA)
	case p.winsB >= need:
		return s.finishPairing(ctx, tx, st, p, *p.botB)
	}
	mid, err := s.newTournamentMatch(ctx, tx, st, p, game+1)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO tanks_tournament_games (pairing_id, game, match_id) VALUES ($1, $2, $3)`, p.id, game+1, mid)
	return err
}

func (s *Service) finishPairing(ctx context.Context, tx pgx.Tx, st *bracketState, p *pairingState, winner string) error {
	p.status, p.winner = "finished", &winner
	_, err := tx.Exec(ctx, `UPDATE tanks_tournament_pairings SET status = 'finished', winner_bot_id = $2 WHERE id = $1`, p.id, winner)
	return err
}

func betterSeed(st *bracketState, a, b string) string {
	if st.seeds[a] <= st.seeds[b] {
		return a
	}
	return b
}

// matchWinner decides a finished 1v1: better place, then more damage, then more kills, then the better seed.
func (s *Service) matchWinner(ctx context.Context, tx pgx.Tx, st *bracketState, matchID string) (string, error) {
	rows, err := tx.Query(ctx, `SELECT bot_id, coalesce(place, 99), damage, kills FROM match_players WHERE match_id = $1 ORDER BY slot`, matchID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	type res struct {
		bot                 string
		place, damage, kill int
	}
	var rs []res
	for rows.Next() {
		var r res
		if err := rows.Scan(&r.bot, &r.place, &r.damage, &r.kill); err != nil {
			return "", err
		}
		rs = append(rs, r)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(rs) != 2 {
		return "", fmt.Errorf("tournament match %s has %d players", matchID, len(rs))
	}
	a, b := rs[0], rs[1]
	switch {
	case a.place != b.place:
		if a.place < b.place {
			return a.bot, nil
		}
		return b.bot, nil
	case a.damage != b.damage:
		if a.damage > b.damage {
			return a.bot, nil
		}
		return b.bot, nil
	case a.kill != b.kill:
		if a.kill > b.kill {
			return a.bot, nil
		}
		return b.bot, nil
	}
	return betterSeed(st, a.bot, b.bot), nil
}

// newTournamentMatch queues game gameNo of a pairing: a 1v1 match on a map that differs from the series'
// other games, the bots swapping slots every game, and its run_match job.
func (s *Service) newTournamentMatch(ctx context.Context, tx pgx.Tx, st *bracketState, p *pairingState, gameNo int) (string, error) {
	maps := tanks.Maps()
	m := maps[(p.round+p.pos+gameNo)%len(maps)]
	seed := rand.Int64N(1 << 53)
	ticks := tanks.DefaultRules().Ticks
	id := idgen.New("match")
	if _, err := tx.Exec(ctx, `INSERT INTO matches (id, game, kind, status, seed, map, ticks) VALUES ($1, $2, 'tournament', 'queued', $3, $4, $5)`,
		id, Game, seed, m.Name, ticks); err != nil {
		return "", err
	}
	first, second := *p.botA, *p.botB
	if gameNo%2 == 0 {
		first, second = second, first
	}
	for slot, bot := range []string{first, second} {
		if _, err := tx.Exec(ctx, `INSERT INTO match_players (match_id, slot, bot_id, version_id) VALUES ($1, $2, $3, $4)`,
			id, slot, bot, st.versions[bot]); err != nil {
			return "", err
		}
	}
	if _, err := jobs.Enqueue(ctx, tx, "run_match", map[string]string{"match_id": id}, "match:"+id); err != nil {
		return "", err
	}
	return id, nil
}

// --- reads ---

const tournamentCols = `t.id, t.name, t.season_id, t.status, t.starts_at, t.started_at, t.finished_at, t.size, t.rounds, t.best_of,
	t.champion_bot_id, (SELECT count(*) FROM tanks_tournament_entries e WHERE e.tournament_id = t.id)`

func scanTournaments(ctx context.Context, tx pgx.Tx, rows pgx.Rows) ([]TournamentView, error) {
	defer rows.Close()
	var out []TournamentView
	var champs []*string
	for rows.Next() {
		var t TournamentView
		var champ *string
		if err := rows.Scan(&t.ID, &t.Name, &t.SeasonID, &t.Status, &t.StartsAt, &t.StartedAt, &t.FinishedAt,
			&t.Size, &t.Rounds, &t.BestOf, &champ, &t.EntryCount); err != nil {
			return nil, err
		}
		t.Open = strings.HasPrefix(t.Name, openTournamentPrefix)
		t.StartsAt = t.StartsAt.UTC()
		if t.StartedAt != nil {
			u := t.StartedAt.UTC()
			t.StartedAt = &u
		}
		if t.FinishedAt != nil {
			u := t.FinishedAt.UTC()
			t.FinishedAt = &u
		}
		t.Now = time.Now().UTC()
		out = append(out, t)
		champs = append(champs, champ)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i, c := range champs {
		if c == nil {
			continue
		}
		b, err := tournamentBotTx(ctx, tx, out[i].ID, *c)
		if err != nil {
			return nil, err
		}
		out[i].Champion = &b
	}
	return out, nil
}

func tournamentBotTx(ctx context.Context, tx pgx.Tx, tournamentID, botID string) (TournamentBot, error) {
	var b TournamentBot
	err := tx.QueryRow(ctx, `SELECT g.id, g.name, g.house, coalesce(e.seed, 0), coalesce(u.handle, '')
		FROM game_bots g
		LEFT JOIN tanks_tournament_entries e ON e.tournament_id = $1 AND e.bot_id = g.id
		LEFT JOIN users u ON u.id = g.owner_user_id
		WHERE g.id = $2`, tournamentID, botID).Scan(&b.BotID, &b.Name, &b.House, &b.Seed, &b.Owner)
	return b, err
}

// tournamentDetailTx loads one tournament with its entrants, bracket and every match of every series.
func (s *Service) tournamentDetailTx(ctx context.Context, tx pgx.Tx, id string) (TournamentView, error) {
	rows, err := tx.Query(ctx, `SELECT `+tournamentCols+` FROM tanks_tournaments t WHERE t.id = $1`, id)
	if err != nil {
		return TournamentView{}, err
	}
	list, err := scanTournaments(ctx, tx, rows)
	if err != nil {
		return TournamentView{}, err
	}
	if len(list) == 0 {
		return TournamentView{}, pgx.ErrNoRows
	}
	t := list[0]

	erows, err := tx.Query(ctx, `SELECT g.id, g.name, g.house, e.seed, coalesce(u.handle, '')
		FROM tanks_tournament_entries e JOIN game_bots g ON g.id = e.bot_id
		LEFT JOIN users u ON u.id = g.owner_user_id
		WHERE e.tournament_id = $1 ORDER BY e.seed`, id)
	if err != nil {
		return TournamentView{}, err
	}
	t.Entries = []TournamentBot{}
	byID := map[string]*TournamentBot{}
	for erows.Next() {
		var b TournamentBot
		if err := erows.Scan(&b.BotID, &b.Name, &b.House, &b.Seed, &b.Owner); err != nil {
			erows.Close()
			return TournamentView{}, err
		}
		t.Entries = append(t.Entries, b)
	}
	erows.Close()
	if err := erows.Err(); err != nil {
		return TournamentView{}, err
	}
	for i := range t.Entries {
		byID[t.Entries[i].BotID] = &t.Entries[i]
	}

	prows, err := tx.Query(ctx, `SELECT id, round, position, bot_a, bot_b, wins_a, wins_b, status, winner_bot_id, bye
		FROM tanks_tournament_pairings WHERE tournament_id = $1 ORDER BY round, position`, id)
	if err != nil {
		return TournamentView{}, err
	}
	t.Pairings = []Pairing{}
	for prows.Next() {
		var p Pairing
		var a, b *string
		if err := prows.Scan(&p.ID, &p.Round, &p.Position, &a, &b, &p.WinsA, &p.WinsB, &p.Status, &p.WinnerBotID, &p.Bye); err != nil {
			prows.Close()
			return TournamentView{}, err
		}
		if a != nil {
			p.A = byID[*a]
		}
		if b != nil {
			p.B = byID[*b]
		}
		p.Games = []TournamentGame{}
		t.Pairings = append(t.Pairings, p)
	}
	prows.Close()
	if err := prows.Err(); err != nil {
		return TournamentView{}, err
	}

	grows, err := tx.Query(ctx, `SELECT g.pairing_id, g.game, g.match_id, m.status, m.map, g.winner_bot_id,
			EXISTS (SELECT 1 FROM match_replays r WHERE r.match_id = m.id)
		FROM tanks_tournament_games g
		JOIN tanks_tournament_pairings p ON p.id = g.pairing_id
		JOIN matches m ON m.id = g.match_id
		WHERE p.tournament_id = $1 ORDER BY g.game`, id)
	if err != nil {
		return TournamentView{}, err
	}
	defer grows.Close()
	idx := map[string]int{}
	for i, p := range t.Pairings {
		idx[p.ID] = i
	}
	for grows.Next() {
		var pid string
		var g TournamentGame
		if err := grows.Scan(&pid, &g.Game, &g.MatchID, &g.Status, &g.Map, &g.WinnerBotID, &g.HasReplay); err != nil {
			return TournamentView{}, err
		}
		t.Pairings[idx[pid]].Games = append(t.Pairings[idx[pid]].Games, g)
	}
	return t, grows.Err()
}

// Tournament returns one tournament with its bracket; 404 not_found if it doesn't exist.
func (s *Service) Tournament(ctx context.Context, id string) (TournamentView, error) {
	var out TournamentView
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = s.tournamentDetailTx(ctx, tx, id)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return TournamentView{}, httpx.NotFound()
	}
	return out, err
}

// Tournaments lists tournaments (scheduled, running and finished), newest first, optionally of one status.
func (s *Service) Tournaments(ctx context.Context, status string, limit int) ([]TournamentView, error) {
	var out []TournamentView
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+tournamentCols+` FROM tanks_tournaments t
			WHERE t.status <> 'cancelled' AND ($1 = '' OR t.status = $1)
			ORDER BY t.starts_at DESC LIMIT $2`, status, limit)
		if err != nil {
			return err
		}
		out, err = scanTournaments(ctx, tx, rows)
		return err
	})
	if out == nil {
		out = []TournamentView{}
	}
	return out, err
}

// Showcase assembles the /tanks front page.
func (s *Service) Showcase(ctx context.Context) (Showcase, error) {
	var out Showcase
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		if out.Season, err = ensureSeasonTx(ctx, tx); err != nil {
			return err
		}
		if out.Ladder, err = seasonLadder(ctx, tx, out.Season.ID, false); err != nil {
			return err
		}
		if len(out.Ladder) > 10 {
			out.Ladder = out.Ladder[:10]
		}

		var liveID string
		err = tx.QueryRow(ctx, `SELECT id FROM tanks_tournaments
			WHERE name NOT LIKE $1 AND (status = 'running' OR (status = 'finished' AND finished_at > now() - interval '1 day'))
			ORDER BY (status = 'running') DESC, coalesce(finished_at, started_at) DESC LIMIT 1`, openTournamentPrefix+"%").Scan(&liveID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if liveID != "" {
			t, err := s.tournamentDetailTx(ctx, tx, liveID)
			if err != nil {
				return err
			}
			out.Tournament = &t
		}

		rows, err := tx.Query(ctx, `SELECT `+tournamentCols+` FROM tanks_tournaments t
			WHERE t.status = 'scheduled' AND t.name NOT LIKE $1 ORDER BY t.starts_at LIMIT 1`, openTournamentPrefix+"%")
		if err != nil {
			return err
		}
		next, err := scanTournaments(ctx, tx, rows)
		if err != nil {
			return err
		}
		if len(next) > 0 {
			out.NextTournament = &next[0]
		}

		rows, err = tx.Query(ctx, `SELECT `+tournamentCols+` FROM tanks_tournaments t
			WHERE t.name LIKE $1 AND (t.status = 'running' OR (t.status = 'finished' AND t.finished_at > now() - interval '1 day'))
			ORDER BY t.starts_at DESC LIMIT 3`, openTournamentPrefix+"%")
		if err != nil {
			return err
		}
		if out.OpenTournaments, err = scanTournaments(ctx, tx, rows); err != nil {
			return err
		}

		rows, err = tx.Query(ctx, `SELECT `+tournamentCols+` FROM tanks_tournaments t
			WHERE t.status = 'finished' AND t.name NOT LIKE $1 ORDER BY t.finished_at DESC LIMIT 5`, openTournamentPrefix+"%")
		if err != nil {
			return err
		}
		if out.Champions, err = scanTournaments(ctx, tx, rows); err != nil {
			return err
		}

		mrows, err := tx.Query(ctx, `SELECT m.id FROM matches m
			WHERE m.game = $1 AND m.status = 'finished' AND (m.kind = 'tournament' OR m.featured)
			  AND EXISTS (SELECT 1 FROM match_replays r WHERE r.match_id = m.id)
			ORDER BY m.finished_at DESC LIMIT 6`, Game)
		if err != nil {
			return err
		}
		var ids []string
		for mrows.Next() {
			var id string
			if err := mrows.Scan(&id); err != nil {
				mrows.Close()
				return err
			}
			ids = append(ids, id)
		}
		mrows.Close()
		if err := mrows.Err(); err != nil {
			return err
		}
		out.Notable = []MatchView{}
		for _, id := range ids {
			mv, err := s.matchViewTx(ctx, tx, id)
			if err != nil {
				return err
			}
			out.Notable = append(out.Notable, mv)
		}

		srows, err := tx.Query(ctx, seasonSelect+` WHERE s.status = 'archived' ORDER BY s.starts_at DESC LIMIT 3`)
		if err != nil {
			return err
		}
		out.PastSeasons, err = scanSeasons(srows)
		return err
	})
	if err != nil {
		return Showcase{}, err
	}
	if out.Champions == nil {
		out.Champions = []TournamentView{}
	}
	if out.OpenTournaments == nil {
		out.OpenTournaments = []TournamentView{}
	}
	if out.Ladder == nil {
		out.Ladder = []LeaderboardEntry{}
	}
	out.Now = time.Now().UTC()
	return out, nil
}

// botTournamentsTx lists the tournaments a bot entered, newest first, with how far it got.
func botTournamentsTx(ctx context.Context, tx pgx.Tx, botID string) ([]BotTournament, error) {
	rows, err := tx.Query(ctx, `
		SELECT t.id, t.name, t.status, t.starts_at, t.rounds, e.seed, coalesce(t.champion_bot_id = e.bot_id, false),
		  (SELECT max(p.round) FROM tanks_tournament_pairings p
		     WHERE p.tournament_id = t.id AND p.status = 'finished' AND (p.bot_a = e.bot_id OR p.bot_b = e.bot_id)
		       AND p.winner_bot_id IS DISTINCT FROM e.bot_id)
		FROM tanks_tournament_entries e JOIN tanks_tournaments t ON t.id = e.tournament_id
		WHERE e.bot_id = $1 AND t.status IN ('running', 'finished')
		ORDER BY t.starts_at DESC LIMIT 20`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BotTournament{}
	for rows.Next() {
		var b BotTournament
		var lost *int
		if err := rows.Scan(&b.TournamentID, &b.Name, &b.Status, &b.StartsAt, &b.Rounds, &b.Seed, &b.Champion, &lost); err != nil {
			return nil, err
		}
		b.StartsAt = b.StartsAt.UTC()
		b.Open = strings.HasPrefix(b.Name, openTournamentPrefix)
		b.Result = tournamentResult(b.Champion, lost, b.Rounds)
		out = append(out, b)
	}
	return out, rows.Err()
}

// tournamentResult words how far a bot got: lostRound is the round it was knocked out in, nil if it still is in.
func tournamentResult(champion bool, lostRound *int, rounds int) string {
	switch {
	case champion:
		return "Champion"
	case lostRound == nil:
		return "In progress"
	case *lostRound == rounds:
		return "Runner-up"
	case *lostRound == rounds-1:
		return "Semifinalist"
	case *lostRound == rounds-2:
		return "Quarterfinalist"
	}
	return fmt.Sprintf("Out in round %d", *lostRound)
}
