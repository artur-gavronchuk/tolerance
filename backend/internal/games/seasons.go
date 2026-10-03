package games

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/rating"
	"tolerance/internal/platform/httpx"
)

// A season is one calendar month in UTC. Ratings on the ladder are per season (tanks_season_ratings);
// game_bots.mu/sigma stay as the lifetime rating. The season rows are created lazily (whoever needs the
// current season inserts it) and archived by SeasonTick, which freezes the final standings.

// SeasonWinner is the first place of an archived season.
type SeasonWinner struct {
	BotID  string `json:"bot_id"`
	Name   string `json:"name"`
	Owner  string `json:"owner"`
	Rating int    `json:"rating"`
}

// SeasonView is one season's window and state.
type SeasonView struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	StartsAt time.Time     `json:"starts_at"`
	EndsAt   time.Time     `json:"ends_at"`
	Status   string        `json:"status"` // active | archived
	Winner   *SeasonWinner `json:"winner"`
}

// SeasonDetail is a season with its standings: frozen for an archived one, live for the current one.
type SeasonDetail struct {
	Season    SeasonView         `json:"season"`
	Standings []LeaderboardEntry `json:"standings"`
	Total     int                `json:"total"` // bots in the standings; the HTTP route may return only the top ones
	Now       time.Time          `json:"now"`
}

// seasonBounds returns the id and window of the season t falls in.
func seasonBounds(t time.Time) (id string, start, end time.Time) {
	t = t.UTC()
	start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start.Format("2006-01"), start, start.AddDate(0, 1, 0)
}

func newSeasonView(id string, start, end time.Time, status string) SeasonView {
	return SeasonView{ID: id, Name: start.UTC().Format("January 2006"), StartsAt: start.UTC(), EndsAt: end.UTC(), Status: status}
}

// ensureSeasonTx makes sure the row of the season the clock is in exists, and returns it.
func ensureSeasonTx(ctx context.Context, tx pgx.Tx) (SeasonView, error) {
	id, start, end := seasonBounds(time.Now())
	if _, err := tx.Exec(ctx, `INSERT INTO tanks_seasons (id, starts_at, ends_at) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`,
		id, start, end); err != nil {
		return SeasonView{}, err
	}
	return newSeasonView(id, start, end, "active"), nil
}

// seasonLadder ranks every bot with an active version (except the idle house bot) by its rating in the
// season; a bot with no rating row yet counts as a fresh one. playedOnly drops bots with no match in the
// season.
func seasonLadder(ctx context.Context, tx pgx.Tx, seasonID string, playedOnly bool) ([]LeaderboardEntry, error) {
	q := `
		SELECT g.id, g.name, g.house, v.source, v.number, g.mu, g.sigma,
		       coalesce(sr.mu, $2::float8), coalesce(sr.sigma, $3::float8), coalesce(sr.matches, 0), coalesce(sr.wins, 0),
		       coalesce(u.handle, '')
		FROM game_bots g
		JOIN bot_versions v ON v.id = g.active_version_id
		LEFT JOIN tanks_season_ratings sr ON sr.season_id = $1 AND sr.bot_id = g.id
		LEFT JOIN users u ON u.id = g.owner_user_id
		WHERE g.id <> 'bot_house_idle' AND g.hidden_at IS NULL`
	if playedOnly {
		q += ` AND coalesce(sr.matches, 0) > 0`
	}
	q += ` ORDER BY g.created_at, g.id`
	rows, err := tx.Query(ctx, q, seasonID, rating.DefaultMu, rating.DefaultSigma)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeaderboardEntry{}
	for rows.Next() {
		var e LeaderboardEntry
		var lifeMu, lifeSigma float64
		if err := rows.Scan(&e.BotID, &e.Name, &e.House, &e.Source, &e.Version, &lifeMu, &lifeSigma,
			&e.Mu, &e.Sigma, &e.Matches, &e.Wins, &e.Owner); err != nil {
			return nil, err
		}
		e.Rating = rating.Display(rating.Rating{Mu: e.Mu, Sigma: e.Sigma})
		e.LifetimeRating = rating.Display(rating.Rating{Mu: lifeMu, Sigma: lifeSigma})
		e.Provisional = e.Matches < ProvisionalMatches
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rating != out[j].Rating {
			return out[i].Rating > out[j].Rating
		}
		if out[i].Matches != out[j].Matches {
			return out[i].Matches > out[j].Matches
		}
		// Early in a season everyone sits at the fresh rating; lifetime strength breaks the tie so
		// tournament seeds and the showcase ladder stay meaningful.
		return out[i].LifetimeRating > out[j].LifetimeRating
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out, nil
}

// SeasonTick keeps the season table current: it makes sure the current season exists and archives every
// active season whose window has ended, freezing its final standings. Safe to call from several processes.
func (s *Service) SeasonTick(ctx context.Context) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := ensureSeasonTx(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id FROM tanks_seasons WHERE status = 'active' AND ends_at <= now()
			ORDER BY id FOR UPDATE SKIP LOCKED`)
		if err != nil {
			return err
		}
		var due []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			due = append(due, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range due {
			if err := s.archiveSeasonTx(ctx, tx, id); err != nil {
				return err
			}
			s.log.Info("games: season archived", "season", id)
		}
		return nil
	})
}

func (s *Service) archiveSeasonTx(ctx context.Context, tx pgx.Tx, id string) error {
	ladder, err := seasonLadder(ctx, tx, id, true)
	if err != nil {
		return err
	}
	for _, e := range ladder {
		if _, err := tx.Exec(ctx, `INSERT INTO tanks_season_standings
			(season_id, bot_id, rank, rating, mu, sigma, matches, wins, bot_name, owner, house, source, version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (season_id, bot_id) DO NOTHING`,
			id, e.BotID, e.Rank, e.Rating, e.Mu, e.Sigma, e.Matches, e.Wins, e.Name, e.Owner, e.House, e.Source, e.Version); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE tanks_seasons SET status = 'archived', finalized_at = now() WHERE id = $1`, id)
	return err
}

// CurrentSeason returns the season the clock is in, creating its row if it is the first call of the month.
func (s *Service) CurrentSeason(ctx context.Context) (SeasonView, error) {
	var out SeasonView
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = ensureSeasonTx(ctx, tx)
		return err
	})
	return out, err
}

func scanSeasons(rows pgx.Rows) ([]SeasonView, error) {
	defer rows.Close()
	out := []SeasonView{}
	for rows.Next() {
		var id, status string
		var start, end time.Time
		var wID, wName, wOwner *string
		var wRating *int
		if err := rows.Scan(&id, &start, &end, &status, &wID, &wName, &wOwner, &wRating); err != nil {
			return nil, err
		}
		sv := newSeasonView(id, start, end, status)
		if wID != nil {
			sv.Winner = &SeasonWinner{BotID: *wID, Name: *wName, Owner: *wOwner, Rating: *wRating}
		}
		out = append(out, sv)
	}
	return out, rows.Err()
}

const seasonSelect = `
	SELECT s.id, s.starts_at, s.ends_at, s.status, w.bot_id, w.bot_name, w.owner, w.rating
	FROM tanks_seasons s
	LEFT JOIN tanks_season_standings w ON w.season_id = s.id AND w.rank = 1`

// Seasons lists the current season and the archived ones, newest first.
func (s *Service) Seasons(ctx context.Context, limit int) ([]SeasonView, error) {
	var out []SeasonView
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := ensureSeasonTx(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, seasonSelect+` ORDER BY s.starts_at DESC LIMIT $1`, limit)
		if err != nil {
			return err
		}
		out, err = scanSeasons(rows)
		return err
	})
	return out, err
}

// Season returns one season with its standings. id "current" means the season the clock is in.
func (s *Service) Season(ctx context.Context, id string) (SeasonDetail, error) {
	var out SeasonDetail
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		cur, err := ensureSeasonTx(ctx, tx)
		if err != nil {
			return err
		}
		if id == "current" {
			id = cur.ID
		}
		rows, err := tx.Query(ctx, seasonSelect+` WHERE s.id = $1`, id)
		if err != nil {
			return err
		}
		list, err := scanSeasons(rows)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return pgx.ErrNoRows
		}
		out.Season = list[0]
		if out.Season.Status == "active" {
			// The live ladder lists every active bot, played or not, to match the bot page rank and tournament seeding.
			out.Standings, err = seasonLadder(ctx, tx, id, false)
			return err
		}
		out.Standings, err = frozenStandings(ctx, tx, id)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SeasonDetail{}, httpx.NotFound()
	}
	if err != nil {
		return SeasonDetail{}, err
	}
	out.Now = time.Now().UTC()
	out.Total = len(out.Standings)
	return out, nil
}

func frozenStandings(ctx context.Context, tx pgx.Tx, id string) ([]LeaderboardEntry, error) {
	rows, err := tx.Query(ctx, `
		SELECT st.rank, st.bot_id, st.bot_name, st.owner, st.house, st.source, st.version,
		       st.rating, st.mu, st.sigma, st.matches, st.wins,
		       coalesce(round(1000 + 40 * (g.mu - 3 * g.sigma))::int, st.rating)
		FROM tanks_season_standings st JOIN game_bots g ON g.id = st.bot_id
		WHERE st.season_id = $1 AND g.hidden_at IS NULL ORDER BY st.rank`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeaderboardEntry{}
	for rows.Next() {
		var e LeaderboardEntry
		if err := rows.Scan(&e.Rank, &e.BotID, &e.Name, &e.Owner, &e.House, &e.Source, &e.Version,
			&e.Rating, &e.Mu, &e.Sigma, &e.Matches, &e.Wins, &e.LifetimeRating); err != nil {
			return nil, err
		}
		e.Provisional = e.Matches < ProvisionalMatches
		out = append(out, e)
	}
	return out, rows.Err()
}

// BotSeasonResult is one archived season of a bot, for its profile.
type BotSeasonResult struct {
	SeasonID string `json:"season_id"`
	Name     string `json:"name"`
	Rank     int    `json:"rank"`
	Rating   int    `json:"rating"`
	Matches  int    `json:"matches"`
	Wins     int    `json:"wins"`
}

func botSeasonResultsTx(ctx context.Context, tx pgx.Tx, botID string) ([]BotSeasonResult, error) {
	rows, err := tx.Query(ctx, `
		SELECT st.season_id, s.starts_at, st.rank, st.rating, st.matches, st.wins
		FROM tanks_season_standings st JOIN tanks_seasons s ON s.id = st.season_id
		WHERE st.bot_id = $1 ORDER BY s.starts_at DESC LIMIT 12`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BotSeasonResult{}
	for rows.Next() {
		var r BotSeasonResult
		var start time.Time
		if err := rows.Scan(&r.SeasonID, &start, &r.Rank, &r.Rating, &r.Matches, &r.Wins); err != nil {
			return nil, err
		}
		r.Name = start.UTC().Format("January 2006")
		out = append(out, r)
	}
	return out, rows.Err()
}
