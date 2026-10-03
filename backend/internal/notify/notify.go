// Package notify is the on-site notification list (the bell in the header). Notifications are not written by the
// features they are about: on read, this package derives candidates from existing data (submissions, the daily
// board, product tasks, tournaments, the tanks ladder) and stores each once per (person, natural key). A row is a
// type plus params; the frontend words it (en/ru). It only reads other packages' tables.
package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games"
	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/products"
	"tolerance/internal/recap"
)

const (
	listLimit = 30
	// syncEvery is the least time between two derivations for one person: the header polls.
	syncEvery = 45 * time.Second
	// recent is how far back an event still produces a notification, so a first visit does not replay history.
	recent = 14 * 24 * time.Hour
	// dropPlaces is how many places the person's bot must fall in the season ladder to be told.
	dropPlaces = 3
)

type Service struct {
	pool     *db.Pool
	games    *games.Service
	products *products.Service
	recap    *recap.Service

	mu     sync.Mutex
	synced map[string]time.Time
}

func NewService(pool *db.Pool, g *games.Service, p *products.Service, r *recap.Service) *Service {
	return &Service{pool: pool, games: g, products: p, recap: r, synced: map[string]time.Time{}}
}

type Notification struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Params    json.RawMessage `json:"params"`
	CreatedAt time.Time       `json:"created_at"`
	Read      bool            `json:"read"`
}

type List struct {
	Items  []Notification `json:"items"`
	Unread int            `json:"unread"`
}

func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/me/notifications", func(w http.ResponseWriter, r *http.Request) {
		l, err := s.List(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, l)
	})
	mux.HandleFunc("POST /api/v1/me/notifications/read", func(w http.ResponseWriter, r *http.Request) {
		if err := s.MarkRead(r.Context(), identity.MustFromContext(r.Context()).UserID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

type candidate struct {
	key    string
	typ    string
	params map[string]any
	at     time.Time
}

func (s *Service) List(ctx context.Context, userID string) (List, error) {
	s.mu.Lock()
	due := time.Since(s.synced[userID]) >= syncEvery
	if due {
		s.synced[userID] = time.Now()
	}
	s.mu.Unlock()
	if due {
		if err := s.sync(ctx, userID); err != nil {
			s.mu.Lock()
			delete(s.synced, userID)
			s.mu.Unlock()
			return List{}, err
		}
	}
	out := List{Items: []Notification{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&out.Unread); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id, type, params, created_at, read_at IS NOT NULL FROM notifications WHERE user_id = $1 ORDER BY created_at DESC, id LIMIT $2`, userID, listLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var n Notification
			if err := rows.Scan(&n.ID, &n.Type, &n.Params, &n.CreatedAt, &n.Read); err != nil {
				return err
			}
			n.CreatedAt = n.CreatedAt.UTC()
			out.Items = append(out.Items, n)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) MarkRead(ctx context.Context, userID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, userID)
		return err
	})
}

// sync derives every candidate and stores the new ones. Each source is independent: one failing does not hide the others.
func (s *Service) sync(ctx context.Context, userID string) error {
	var cands []candidate
	for _, src := range []func(context.Context, string) ([]candidate, error){s.verdicts, s.yesterday, s.productPhases, s.tanks} {
		c, err := src(ctx, userID)
		if err != nil {
			return err
		}
		cands = append(cands, c...)
	}
	if len(cands) == 0 {
		return nil
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, c := range cands {
			params, err := json.Marshal(c.params)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO notifications (id, user_id, key, type, params, created_at) VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (user_id, key) DO NOTHING`, idgen.New("ntf"), userID, c.key, c.typ, params, c.at.UTC()); err != nil {
				return err
			}
		}
		return nil
	})
}

// verdicts: a daily submission of the last days has finished (including infra_error, which costs no attempt).
func (s *Service) verdicts(ctx context.Context, userID string) ([]candidate, error) {
	var out []candidate
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT s.id, s.day::text, t.title, t.kind, s.status, s.passed_tests, s.total_tests, s.score, s.finished_at
			FROM submissions s JOIN tasks t ON t.slug = s.task_slug
			WHERE s.user_id = $1 AND s.day IS NOT NULL AND s.status IN ('passed', 'failed', 'infra_error')
			  AND s.finished_at > now() - interval '3 days'`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, day, title, kind, status string
			var passed, total int
			var score *float64
			var at time.Time
			if err := rows.Scan(&id, &day, &title, &kind, &status, &passed, &total, &score, &at); err != nil {
				return err
			}
			out = append(out, candidate{"verdict:" + id, "daily_verdict", map[string]any{
				"day": day, "title": title, "kind": kind, "status": status, "passed": passed, "total": total, "score": score,
			}, at})
		}
		return rows.Err()
	})
	return out, err
}

// yesterday: the day closed and the person has a place on its board.
func (s *Service) yesterday(ctx context.Context, userID string) ([]candidate, error) {
	y, err := s.recap.YesterdayOf(ctx, userID)
	if err != nil || y == nil || y.Place == nil {
		return nil, err
	}
	day, _ := time.Parse("2006-01-02", y.Day)
	return []candidate{{"day_final:" + y.Day, "daily_final", map[string]any{
		"day": y.Day, "title": y.Task.Title, "place": *y.Place, "of": y.Participants,
	}, day.Add(24 * time.Hour)}}, nil
}

// productPhases: voting opened (with the pairs this person has to judge) and final standings with the person's place.
func (s *Service) productPhases(ctx context.Context, userID string) ([]candidate, error) {
	l, err := s.products.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []candidate
	for _, t := range l.Items {
		switch {
		case t.Phase == products.PhaseVoting:
			p := map[string]any{"slug": t.Slug, "title": t.Title, "kind": t.Kind, "pairs": 0}
			if t.Kind == products.KindSite {
				n, err := s.products.Next(ctx, userID, t.Slug)
				if err != nil || n.Target-n.Judged <= 0 {
					continue
				}
				p["pairs"] = n.Target - n.Judged
			}
			out = append(out, candidate{"product_voting:" + t.Slug, "product_voting", p, t.Deadline})
		case t.Phase == products.PhaseFinal && time.Since(t.VotingEndsAt) < recent:
			res, err := s.products.Results(ctx, t.Slug, userID)
			if err != nil {
				return nil, err
			}
			for i, e := range res.Entries {
				if e.Mine {
					out = append(out, candidate{"product_final:" + t.Slug, "product_final", map[string]any{
						"slug": t.Slug, "title": t.Title, "place": i + 1, "of": len(res.Entries),
					}, t.VotingEndsAt})
					break
				}
			}
		}
	}
	return out, nil
}

// tanks: tournaments the person's bot is in, one about to start, and a drop in the season ladder.
func (s *Service) tanks(ctx context.Context, userID string) ([]candidate, error) {
	var botID, botName string
	var out []candidate
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id, name FROM game_bots WHERE owner_user_id = $1 AND game = 'tanks' ORDER BY created_at LIMIT 1`, userID).Scan(&botID, &botName)
		if err == pgx.ErrNoRows {
			botID = ""
			return nil
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, name, starts_at FROM tanks_tournaments
			WHERE status = 'scheduled' AND starts_at > now() AND starts_at <= now() + interval '1 hour'`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, name string
			var at time.Time
			if err := rows.Scan(&id, &name, &at); err != nil {
				rows.Close()
				return err
			}
			out = append(out, candidate{"tournament_soon:" + id, "tournament_soon", map[string]any{"id": id, "name": name, "starts_at": at.UTC()}, time.Now()})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `
			SELECT t.id, t.name, t.status, t.rounds, coalesce(t.finished_at, t.started_at, t.starts_at), coalesce(t.champion_bot_id = e.bot_id, false),
			  (SELECT max(p.round) FROM tanks_tournament_pairings p
			     WHERE p.tournament_id = t.id AND p.status = 'finished' AND (p.bot_a = e.bot_id OR p.bot_b = e.bot_id)
			       AND p.winner_bot_id IS DISTINCT FROM e.bot_id)
			FROM tanks_tournament_entries e JOIN tanks_tournaments t ON t.id = e.tournament_id
			WHERE e.bot_id = $1 AND t.status IN ('running', 'finished') AND coalesce(t.started_at, t.starts_at) > now() - interval '14 days'`, botID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, name, status string
			var rounds int
			var at time.Time
			var champion bool
			var lost *int
			if err := rows.Scan(&id, &name, &status, &rounds, &at, &champion, &lost); err != nil {
				return err
			}
			p := map[string]any{"id": id, "name": name, "bot": botName}
			out = append(out, candidate{"tournament_entered:" + id, "tournament_entered", p, at})
			if champion {
				out = append(out, candidate{"tournament_won:" + id, "tournament_won", p, at})
			} else if status == "finished" && lost != nil {
				out = append(out, candidate{"tournament_lost:" + id, "tournament_lost", map[string]any{
					"id": id, "name": name, "bot": botName, "round": *lost, "rounds": rounds,
				}, at})
			}
		}
		return rows.Err()
	})
	if err != nil || botID == "" {
		return out, err
	}
	drop, err := s.rankDrop(ctx, userID, botID, botName)
	if err != nil {
		return nil, err
	}
	if drop != nil {
		out = append(out, *drop)
	}
	return out, nil
}

// rankDrop compares the bot's season rank with the best one it held since the last notice. Falling dropPlaces or more
// is a notification and the new rank becomes the baseline; an improvement raises the baseline silently.
func (s *Service) rankDrop(ctx context.Context, userID, botID, botName string) (*candidate, error) {
	cur, err := s.games.Season(ctx, "current")
	if err != nil {
		return nil, err
	}
	rank := 0
	for _, e := range cur.Standings {
		if e.BotID == botID {
			rank = e.Rank
		}
	}
	if rank == 0 {
		return nil, nil
	}
	var out *candidate
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var season string
		var base int
		err := tx.QueryRow(ctx, `SELECT season_id, bot_rank FROM notify_state WHERE user_id = $1`, userID).Scan(&season, &base)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		next := rank
		if err == nil && season == cur.Season.ID && rank > base {
			next = base
			if rank-base >= dropPlaces {
				next = rank
				out = &candidate{"rank_drop:" + season + ":" + strconv.Itoa(base) + ":" + strconv.Itoa(rank), "rank_drop", map[string]any{
					"bot": botName, "season": season, "from": base, "to": rank,
				}, time.Now()}
			}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO notify_state (user_id, season_id, bot_rank) VALUES ($1, $2, $3)
			ON CONFLICT (user_id) DO UPDATE SET season_id = $2, bot_rank = $3, updated_at = now()`, userID, cur.Season.ID, next)
		return err
	})
	return out, err
}
