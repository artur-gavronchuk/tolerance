package products

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/httpx"
)

// Blind comparison is the main way sites are judged: a person is shown two anonymous sites, says which is
// better (or a tie), and everybody's judgments are turned into one strength per site (Bradley-Terry).

// CompareTarget is how many pairs a person is asked to judge per task.
const CompareTarget = 15

// Standing is one counted entry's place in a site task: its Bradley-Terry score and how many judgments it has.
type Standing struct {
	EntryID     string
	UserID      string
	Score       float64
	Comparisons int
	Votes       int
	Passed      int
	CreatedAt   time.Time
}

type judgment struct{ a, b, winner string }

// SiteRanking orders a site task's counted entries (each person's latest upload): Bradley-Terry score, then
// direct votes, then automated checks, then the earlier upload.
func SiteRanking(ctx context.Context, tx pgx.Tx, slug string) ([]Standing, error) {
	rows, err := tx.Query(ctx, `
		SELECT e.id, e.user_id, `+votesOf+`, e.passed, e.created_at FROM (
			SELECT DISTINCT ON (user_id) * FROM product_entries WHERE task_slug = $1 AND status = 'done'
			ORDER BY user_id, created_at DESC) e
		ORDER BY e.created_at, e.id`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var st []Standing
	for rows.Next() {
		var s Standing
		if err := rows.Scan(&s.EntryID, &s.UserID, &s.Votes, &s.Passed, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.CreatedAt = s.CreatedAt.UTC()
		st = append(st, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	js, err := loadJudgments(ctx, tx, slug, "")
	if err != nil {
		return nil, err
	}
	rankBT(st, js)
	return st, nil
}

// rankBT fills Score and Comparisons of st from the judgments and sorts it into standings.
func rankBT(st []Standing, js []judgment) {
	idx := make(map[string]int, len(st))
	for i, s := range st {
		idx[s.EntryID] = i
	}
	n := len(st)
	type game struct {
		i, j int
		w    float64 // i's score: 1 win, 0.5 tie, 0 loss
	}
	var games []game
	for _, j := range js {
		a, okA := idx[j.a]
		b, okB := idx[j.b]
		if !okA || !okB {
			continue
		}
		w := 0.5
		switch j.winner {
		case "a":
			w = 1
		case "b":
			w = 0
		}
		games = append(games, game{a, b, w})
		st[a].Comparisons++
		st[b].Comparisons++
	}
	// Minorization-maximization for Bradley-Terry. Every entry also plays two ties against a phantom of
	// strength 1, which keeps unbeaten and winless entries finite.
	p := make([]float64, n)
	for i := range p {
		p[i] = 1
	}
	for it := 0; it < 1000 && n > 0; it++ {
		wins := make([]float64, n)
		den := make([]float64, n)
		for i := range p {
			wins[i] = 1
			den[i] = 2 / (p[i] + 1)
		}
		for _, g := range games {
			wins[g.i] += g.w
			wins[g.j] += 1 - g.w
			d := 1 / (p[g.i] + p[g.j])
			den[g.i] += d
			den[g.j] += d
		}
		next := make([]float64, n)
		sum := 0.0
		for i := range p {
			next[i] = wins[i] / den[i]
			sum += math.Log(next[i])
		}
		shift := math.Exp(sum / float64(n)) // keep the geometric mean at 1
		delta := 0.0
		for i := range next {
			next[i] /= shift
			delta = math.Max(delta, math.Abs(math.Log(next[i]/p[i])))
		}
		p = next
		if delta < 1e-9 {
			break
		}
	}
	for i := range st {
		// An Elo-like scale: 1000 is the average site, 400 points is a factor of ten in strength.
		st[i].Score = math.Round((1000+400*math.Log10(p[i]))*10) / 10
	}
	sort.SliceStable(st, func(a, b int) bool {
		x, y := st[a], st[b]
		switch {
		case x.Score != y.Score:
			return x.Score > y.Score
		case x.Votes != y.Votes:
			return x.Votes > y.Votes
		case x.Passed != y.Passed:
			return x.Passed > y.Passed
		}
		return x.CreatedAt.Before(y.CreatedAt)
	})
}

// loadJudgments reads a task's judgments; with userID only that person's.
func loadJudgments(ctx context.Context, tx pgx.Tx, slug, userID string) ([]judgment, error) {
	rows, err := tx.Query(ctx, `SELECT entry_a, entry_b, winner FROM product_judgments WHERE task_slug = $1 AND ($2 = '' OR user_id = $2)`, slug, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []judgment
	for rows.Next() {
		var j judgment
		if err := rows.Scan(&j.a, &j.b, &j.winner); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// Anon is an entry in a blind pair: nothing about its author.
type Anon struct {
	ID string `json:"id"`
}

// Pair is two sites to judge.
type Pair struct {
	A Anon `json:"a"`
	B Anon `json:"b"`
}

// NextPair answers GET /compare/next: the pair to judge now (null when the person has judged enough) and their
// progress.
type NextPair struct {
	Pair   *Pair `json:"pair"`
	Judged int   `json:"judged"`
	Target int   `json:"target"`
}

// compareOpen checks the task is a site task in its voting phase.
func compareOpen(ctx context.Context, tx pgx.Tx, slug string) error {
	var kind string
	var deadline time.Time
	if err := tx.QueryRow(ctx, `SELECT kind, deadline FROM product_tasks WHERE slug = $1 AND active AND opens_at <= now()`, slug).Scan(&kind, &deadline); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		return err
	}
	if kind != KindSite {
		return httpx.New(http.StatusConflict, "not_a_site_task", "Only site tasks are judged by comparison")
	}
	return checkVoting(deadline)
}

func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

// progress counts the caller's judgments among the entries they may judge (everyone's but their own) and the
// target for them.
func progress(st []Standing, mine []judgment, userID string) (judged, target int) {
	ok := map[string]bool{}
	for _, e := range st {
		if e.UserID != userID {
			ok[e.EntryID] = true
		}
	}
	for _, j := range mine {
		if ok[j.a] && ok[j.b] {
			judged++
		}
	}
	return judged, min(CompareTarget, len(ok)*(len(ok)-1)/2)
}

// Next picks the pair that teaches the most: not judged by this person yet, sites with few judgments, and
// sites whose current scores are close. Never the person's own site.
func (s *Service) Next(ctx context.Context, userID, slug string) (NextPair, error) {
	var out NextPair
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := compareOpen(ctx, tx, slug); err != nil {
			return err
		}
		st, err := SiteRanking(ctx, tx, slug)
		if err != nil {
			return err
		}
		mine, err := loadJudgments(ctx, tx, slug, userID)
		if err != nil {
			return err
		}
		out.Judged, out.Target = progress(st, mine, userID)
		if out.Judged >= out.Target {
			return nil
		}
		done := map[string]bool{}
		for _, j := range mine {
			done[pairKey(j.a, j.b)] = true
		}
		var pool []Standing
		for _, e := range st {
			if e.UserID != userID {
				pool = append(pool, e)
			}
		}
		best, bestCost := [2]int{-1, -1}, math.Inf(1)
		for i := range pool {
			for j := i + 1; j < len(pool); j++ {
				if done[pairKey(pool[i].EntryID, pool[j].EntryID)] {
					continue
				}
				// Few judgments first, then close scores, then a little noise so people see different pairs.
				cost := float64(pool[i].Comparisons+pool[j].Comparisons) + math.Abs(pool[i].Score-pool[j].Score)/100 + rand.Float64()*1.5
				if cost < bestCost {
					best, bestCost = [2]int{i, j}, cost
				}
			}
		}
		if best[0] < 0 {
			return nil
		}
		a, b := pool[best[0]], pool[best[1]]
		if rand.IntN(2) == 0 {
			a, b = b, a
		}
		out.Pair = &Pair{A: Anon{a.EntryID}, B: Anon{b.EntryID}}
		return nil
	})
	return out, err
}

// Revealed is a judged entry with its author, shown after the judgment.
type Revealed struct {
	ID       string `json:"id"`
	Handle   string `json:"handle"`
	MadeWith string `json:"made_with"`
}

// Judged answers POST /compare: the authors of the pair and the person's progress.
type Judged struct {
	A      Revealed `json:"a"`
	B      Revealed `json:"b"`
	Winner string   `json:"winner"`
	Judged int      `json:"judged"`
	Target int      `json:"target"`
}

// Judge stores the caller's verdict on a pair (replacing an earlier one on the same pair) while voting is open.
func (s *Service) Judge(ctx context.Context, userID, slug, a, b, winner string) (Judged, error) {
	var out Judged
	if winner != "a" && winner != "b" && winner != "tie" {
		return out, httpx.WithField(http.StatusUnprocessableEntity, "invalid_winner", `winner must be "a", "b" or "tie"`, "winner", "invalid")
	}
	if a == b {
		return out, httpx.WithField(http.StatusUnprocessableEntity, "invalid_pair", "Pick two different entries", "b", "invalid")
	}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := compareOpen(ctx, tx, slug); err != nil {
			return err
		}
		st, err := SiteRanking(ctx, tx, slug)
		if err != nil {
			return err
		}
		counted := map[string]Standing{}
		for _, e := range st {
			counted[e.EntryID] = e
		}
		ea, okA := counted[a]
		eb, okB := counted[b]
		if !okA || !okB {
			return httpx.New(http.StatusConflict, "not_counted", "That upload is not the one shown in the results")
		}
		if ea.UserID == userID || eb.UserID == userID {
			return httpx.New(http.StatusForbidden, "own_entry", "You cannot judge your own entry")
		}
		x, y, w := a, b, winner
		if x > y {
			x, y = y, x
			if winner != "tie" {
				w = map[string]string{"a": "b", "b": "a"}[winner]
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO product_judgments (task_slug, user_id, entry_a, entry_b, winner) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (user_id, entry_a, entry_b) DO UPDATE SET winner = EXCLUDED.winner, updated_at = now()`, slug, userID, x, y, w); err != nil {
			return err
		}
		reveal := func(id string) (r Revealed, err error) {
			r.ID = id
			err = tx.QueryRow(ctx, `SELECT u.handle, e.made_with FROM product_entries e JOIN users u ON u.id = e.user_id WHERE e.id = $1`, id).Scan(&r.Handle, &r.MadeWith)
			return
		}
		if out.A, err = reveal(a); err != nil {
			return err
		}
		if out.B, err = reveal(b); err != nil {
			return err
		}
		out.Winner = winner
		mine, err := loadJudgments(ctx, tx, slug, userID)
		if err != nil {
			return err
		}
		out.Judged, out.Target = progress(st, mine, userID)
		return nil
	})
	return out, err
}
