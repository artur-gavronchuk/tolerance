package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// notifications hands the first 60 people a few bell items of different types (the real ones are derived
// lazily from activity; these are only to have rows and an unread count to look at).
func (s *seeder) notifications(ctx context.Context, tx pgx.Tx) error {
	var rows [][]any
	add := func(u *seedUser, key, typ string, params map[string]any, at time.Time, read bool) {
		b, _ := json.Marshal(params)
		var readAt any
		if read {
			readAt = at.Add(time.Hour)
		}
		rows = append(rows, []any{fmt.Sprintf("%sn%d", idPrefix, len(rows)), u.id, key, typ, b, at, readAt})
	}
	for i := 0; i < 60 && i < len(s.usersList); i++ {
		u := &s.usersList[i]
		at := s.now.Add(-time.Duration(1+s.rng.Intn(48)) * time.Hour)
		add(u, "seed:verdict:1", "daily_verdict", map[string]any{"day": s.today.AddDate(0, 0, -1).Format("2006-01-02"), "title": "Fix the retry loop", "kind": "bugfix", "status": "passed", "passed": 5, "total": 5, "score": nil}, at, false)
		add(u, "seed:verdict:2", "daily_verdict", map[string]any{"day": s.today.AddDate(0, 0, -2).Format("2006-01-02"), "title": "Interval merge", "kind": "bugfix", "status": "failed", "passed": 3, "total": 5, "score": nil}, at.Add(-26*time.Hour), true)
		add(u, "seed:final:1", "daily_final", map[string]any{"day": s.today.AddDate(0, 0, -1).Format("2006-01-02"), "title": "Fix the retry loop", "place": 1 + s.rng.Intn(80), "of": 180}, at.Add(-2*time.Hour), false)
		add(u, "seed:product:1", "product_final", map[string]any{"slug": "calc", "title": "calc", "kind": "cli", "pairs": 0, "place": 1 + s.rng.Intn(100), "of": 140}, at.Add(-30*time.Hour), true)
		if u.botID != "" {
			add(u, "seed:tourn:1", "tournament_lost", map[string]any{"id": "seed_t0", "name": "Weekly tournament", "bot": u.handle, "round": 2, "rounds": 3}, at.Add(-20*time.Hour), false)
		}
	}
	s.counts["notifications"] = len(rows)
	return s.copy(ctx, tx, "notifications", []string{"id", "user_id", "key", "type", "params", "created_at", "read_at"}, rows)
}
