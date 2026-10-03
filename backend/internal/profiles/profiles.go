// Package profiles assembles a person's public activity across the two modes (tanks and the "made with" stack) for
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
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

type BotTournament struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	StartsAt time.Time `json:"starts_at"`
	Result   string    `json:"result"`
	Champion bool      `json:"champion"`
	Open     bool      `json:"open"` // started on demand, not the weekly one
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
	Handle string `json:"handle"`
	Stack  *Stack `json:"stack"`
	Bots   []Bot  `json:"bots"`
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
	a := Activity{Bots: []Bot{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var userID string
		err := tx.QueryRow(ctx, `SELECT id, handle FROM users WHERE lower(handle) = lower($1) AND banned_at IS NULL`, handle).Scan(&userID, &a.Handle)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
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
		WHERE g.id <> 'bot_house_idle' AND g.hidden_at IS NULL`, season, rating.DefaultMu, rating.DefaultSigma, userID)
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
		WHERE owner_user_id = $1 AND active_version_id IS NULL AND hidden_at IS NULL ORDER BY created_at`, userID)
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
		b.Open = strings.HasPrefix(b.Name, "Open tournament")
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

// stackOf is the person's most used normalized "made with" across daily submissions. An
// unrecognised stack ("Other") only wins when nothing else was ever named.
func stackOf(ctx context.Context, tx pgx.Tx, userID string) (*Stack, error) {
	rows, err := tx.Query(ctx, `
		SELECT lower(trim(made_with)), count(*) FROM submissions
		WHERE user_id = $1 AND day IS NOT NULL AND hidden_at IS NULL AND trim(made_with) <> '' GROUP BY 1`, userID)
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
		tool, model := Normalize(raw)
		counts[key{tool, model}] += int(n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var best *Stack
	for k, n := range counts {
		s := Stack{Tool: k.tool, Model: k.model, Label: Label(k.tool, k.model), Count: n}
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
