// Package profiles assembles a person's public activity across the three modes (products, tanks, stack) for
// the profile page. It only reads; the daily part of the profile stays in internal/daily.
package profiles

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/rating"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/products"
	"tolerance/internal/stacks"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

type ProductEntry struct {
	TaskSlug  string    `json:"task_slug"`
	TaskTitle string    `json:"task_title"`
	Kind      string    `json:"kind"`
	Phase     string    `json:"phase"`
	Deadline  time.Time `json:"deadline"`
	EntryID   string    `json:"entry_id"`
	Passed    int       `json:"passed"` // automated checks; hidden (0) while the task is open
	Total     int       `json:"total"`
	Votes     int       `json:"votes"`
	Place     *int      `json:"place"`    // final standings only
	Entrants  int       `json:"entrants"` // people in the standings
	CreatedAt time.Time `json:"created_at"`
}

type BotTournament struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	StartsAt time.Time `json:"starts_at"`
	Result   string    `json:"result"`
	Champion bool      `json:"champion"`
}

type Bot struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Rating         int             `json:"rating"` // this season
	Rank           *int            `json:"rank"`   // this season; nil if the bot is not on the ladder
	LifetimeRating int             `json:"lifetime_rating"`
	Matches        int             `json:"matches"` // this season
	Wins           int             `json:"wins"`
	TotalMatches   int             `json:"total_matches"`
	TotalWins      int             `json:"total_wins"`
	Titles         int             `json:"titles"`
	BestFinish     string          `json:"best_finish"`
	Tournaments    []BotTournament `json:"tournaments"`
}

type Stack struct {
	Tool  string `json:"tool"`
	Model string `json:"model"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type Activity struct {
	Handle   string         `json:"handle"`
	Stack    *Stack         `json:"stack"`
	Products []ProductEntry `json:"products"`
	Bots     []Bot          `json:"bots"`
}

func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/users/{handle}/activity", func(w http.ResponseWriter, r *http.Request) {
		a, err := s.Activity(r.Context(), r.PathValue("handle"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, a)
	})
}

func (s *Service) Activity(ctx context.Context, handle string) (Activity, error) {
	a := Activity{Products: []ProductEntry{}, Bots: []Bot{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var userID string
		err := tx.QueryRow(ctx, `SELECT id, handle FROM users WHERE lower(handle) = lower($1)`, handle).Scan(&userID, &a.Handle)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		if a.Products, err = productsOf(ctx, tx, userID); err != nil {
			return err
		}
		if a.Bots, err = botsOf(ctx, tx, userID); err != nil {
			return err
		}
		a.Stack, err = stackOf(ctx, tx, userID)
		return err
	})
	return a, err
}

// productsOf lists the tasks the person took part in with the upload that counts for them. The counting rule and the
// ranking rule mirror products.rankRule (cli: best upload, ranked by checks then votes). Site tasks take their place from
// products.SiteRanking (Bradley-Terry score of the blind comparisons). Entries stay hidden until the deadline, so open
// tasks carry no score.
func productsOf(ctx context.Context, tx pgx.Tx, userID string) ([]ProductEntry, error) {
	rows, err := tx.Query(ctx, `
		WITH counted AS (
			SELECT DISTINCT ON (e.task_slug, e.user_id) e.id, e.task_slug, e.user_id, e.passed, e.total, e.created_at,
			       t.kind, t.title, t.deadline,
			       (SELECT count(*) FROM product_votes v WHERE v.entry_id = e.id) AS votes
			FROM product_entries e JOIN product_tasks t ON t.slug = e.task_slug
			WHERE e.status = 'done' AND t.active AND t.opens_at <= now()
			  AND e.task_slug IN (SELECT task_slug FROM product_entries WHERE user_id = $1 AND status = 'done')
			ORDER BY e.task_slug, e.user_id,
			         (CASE WHEN t.kind = 'cli' THEN e.passed END) DESC NULLS LAST,
			         (CASE WHEN t.kind = 'site' THEN e.created_at END) DESC NULLS LAST,
			         e.created_at),
		ranked AS (
			SELECT c.*, row_number() OVER (PARTITION BY c.task_slug ORDER BY
			         (CASE WHEN c.kind = 'cli' THEN c.passed ELSE c.votes END) DESC,
			         (CASE WHEN c.kind = 'cli' THEN c.votes ELSE c.passed END) DESC, c.created_at) AS place,
			       count(*) OVER (PARTITION BY c.task_slug) AS entrants
			FROM counted c)
		SELECT task_slug, title, kind, deadline, id, passed, total, votes, place, entrants, created_at
		FROM ranked WHERE user_id = $1 ORDER BY deadline DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProductEntry{}
	for rows.Next() {
		var p ProductEntry
		var votes, place, entrants int64
		if err := rows.Scan(&p.TaskSlug, &p.TaskTitle, &p.Kind, &p.Deadline, &p.EntryID, &p.Passed, &p.Total, &votes, &place, &entrants, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Deadline, p.CreatedAt = p.Deadline.UTC(), p.CreatedAt.UTC()
		p.Votes, p.Entrants = int(votes), int(entrants)
		p.Phase = phaseOf(p.Deadline)
		if p.Phase == products.PhaseOpen {
			p.Passed, p.Total, p.Votes, p.Entrants = 0, 0, 0, 0
		}
		if p.Phase == products.PhaseFinal {
			pl := int(place)
			p.Place = &pl
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i, p := range out {
		if p.Kind != products.KindSite || p.Phase == products.PhaseOpen {
			continue
		}
		st, err := products.SiteRanking(ctx, tx, p.TaskSlug)
		if err != nil {
			return nil, err
		}
		for n, s := range st {
			if s.EntryID == p.EntryID {
				if p.Phase == products.PhaseFinal {
					pl := n + 1
					out[i].Place = &pl
				}
				break
			}
		}
	}
	return out, nil
}

func phaseOf(deadline time.Time) string {
	now := time.Now()
	switch {
	case now.Before(deadline):
		return products.PhaseOpen
	case now.Before(deadline.Add(products.VotingWindow)):
		return products.PhaseVoting
	}
	return products.PhaseFinal
}

// botsOf lists the person's tank bots with their rank on this month's ladder (same ordering as the leaderboard:
// season rating, then matches, then lifetime rating) and how they did in tournaments.
func botsOf(ctx context.Context, tx pgx.Tx, userID string) ([]Bot, error) {
	now := time.Now().UTC()
	season := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
	rows, err := tx.Query(ctx, `
		SELECT g.id, g.name, coalesce(g.owner_user_id = $4, false), g.mu, g.sigma, g.matches, g.wins,
		       coalesce(sr.mu, $2::float8), coalesce(sr.sigma, $3::float8), coalesce(sr.matches, 0), coalesce(sr.wins, 0)
		FROM game_bots g
		JOIN bot_versions v ON v.id = g.active_version_id
		LEFT JOIN tanks_season_ratings sr ON sr.season_id = $1 AND sr.bot_id = g.id
		WHERE g.id <> 'bot_house_idle'`, season, rating.DefaultMu, rating.DefaultSigma, userID)
	if err != nil {
		return nil, err
	}
	type ranked struct {
		b    Bot
		mine bool
	}
	var all []ranked
	for rows.Next() {
		var x ranked
		var mu, sigma, smu, ssigma float64
		if err := rows.Scan(&x.b.ID, &x.b.Name, &x.mine, &mu, &sigma, &x.b.TotalMatches, &x.b.TotalWins, &smu, &ssigma, &x.b.Matches, &x.b.Wins); err != nil {
			rows.Close()
			return nil, err
		}
		x.b.Rating = rating.Display(rating.Rating{Mu: smu, Sigma: ssigma})
		x.b.LifetimeRating = rating.Display(rating.Rating{Mu: mu, Sigma: sigma})
		all = append(all, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i].b, all[j].b
		if a.Rating != b.Rating {
			return a.Rating > b.Rating
		}
		if a.Matches != b.Matches {
			return a.Matches > b.Matches
		}
		return a.LifetimeRating > b.LifetimeRating
	})
	out := []Bot{}
	for i, x := range all {
		if !x.mine {
			continue
		}
		rank := i + 1
		x.b.Rank = &rank
		out = append(out, x.b)
	}
	// An owned bot with no active version is not on the ladder; list it without a rank.
	extra, err := tx.Query(ctx, `
		SELECT id, name, mu, sigma, matches, wins FROM game_bots
		WHERE owner_user_id = $1 AND active_version_id IS NULL ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	for extra.Next() {
		var b Bot
		var mu, sigma float64
		if err := extra.Scan(&b.ID, &b.Name, &mu, &sigma, &b.TotalMatches, &b.TotalWins); err != nil {
			extra.Close()
			return nil, err
		}
		b.Rating = rating.Display(rating.Rating{Mu: mu, Sigma: sigma})
		b.LifetimeRating = b.Rating
		out = append(out, b)
	}
	extra.Close()
	if err := extra.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Tournaments, err = botTournaments(ctx, tx, out[i].ID); err != nil {
			return nil, err
		}
		out[i].Titles, out[i].BestFinish = summarize(out[i].Tournaments)
	}
	return out, nil
}

func botTournaments(ctx context.Context, tx pgx.Tx, botID string) ([]BotTournament, error) {
	rows, err := tx.Query(ctx, `
		SELECT t.id, t.name, t.starts_at, t.rounds, coalesce(t.champion_bot_id = e.bot_id, false),
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
		var rounds int
		var lost *int
		if err := rows.Scan(&b.ID, &b.Name, &b.StartsAt, &rounds, &b.Champion, &lost); err != nil {
			return nil, err
		}
		b.StartsAt = b.StartsAt.UTC()
		b.Result = finish(b.Champion, lost, rounds)
		out = append(out, b)
	}
	return out, rows.Err()
}

// finish words how far a bot got; the same wording as the tanks bot profile.
func finish(champion bool, lostRound *int, rounds int) string {
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

func summarize(ts []BotTournament) (titles int, best string) {
	order := map[string]int{"Champion": 5, "Runner-up": 4, "Semifinalist": 3, "Quarterfinalist": 2}
	score := -1
	for _, t := range ts {
		if t.Champion {
			titles++
		}
		sc, ok := order[t.Result]
		if !ok {
			if t.Result == "In progress" {
				continue
			}
			sc = 1
		}
		if sc > score {
			score, best = sc, t.Result
		}
	}
	return
}

// stackOf is the person's most used normalized "made with" across daily submissions and product uploads. An
// unrecognised stack ("Other") only wins when nothing else was ever named.
func stackOf(ctx context.Context, tx pgx.Tx, userID string) (*Stack, error) {
	rows, err := tx.Query(ctx, `
		SELECT lower(trim(made_with)), count(*) FROM (
			SELECT made_with FROM submissions WHERE user_id = $1 AND day IS NOT NULL
			UNION ALL SELECT made_with FROM product_entries WHERE user_id = $1 AND status = 'done') m
		WHERE trim(made_with) <> '' GROUP BY 1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type key struct{ tool, model string }
	counts := map[key]int{}
	for rows.Next() {
		var raw string
		var n int64
		if err := rows.Scan(&raw, &n); err != nil {
			return nil, err
		}
		tool, model := stacks.Normalize(raw)
		counts[key{tool, model}] += int(n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var best *Stack
	for k, n := range counts {
		s := Stack{Tool: k.tool, Model: k.model, Label: stacks.Label(k.tool, k.model), Count: n}
		if best == nil || better(s, *best) {
			best = &s
		}
	}
	return best, nil
}

func better(a, b Stack) bool {
	ao, bo := a.Label == "Other", b.Label == "Other"
	if ao != bo {
		return !ao
	}
	if a.Count != b.Count {
		return a.Count > b.Count
	}
	return strings.Compare(a.Label, b.Label) < 0
}
