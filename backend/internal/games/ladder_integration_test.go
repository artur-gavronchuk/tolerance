package games_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games"
	"tolerance/internal/games/tanks"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/idgen"
)

const idlePy = `import tanks


class Idle:
    def decide(self, msg):
        return {"move": 0, "turn": 0, "turret": 0, "fire": False}


if __name__ == "__main__":
    tanks.run(Idle())
`

// newUserBotWithArchive signs up a fresh owner, creates their bot and force-activates archive as its
// version directly (bypassing Qualify, which is exercised on its own in qualify_integration_test.go) so
// ladder tests can set up an active user bot quickly and deterministically.
func newUserBotWithArchive(t *testing.T, f fixture, email, name string, archive []byte) (userID, botID string) {
	t.Helper()
	ctx := context.Background()
	u, _, err := f.users.SignIn(ctx, identity.Identity{Provider: "dev", Subject: email, Email: email, EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	m, err := f.svc.SaveBot(ctx, u.ID, name)
	if err != nil {
		t.Fatal(err)
	}
	v, err := f.svc.UploadVersion(ctx, u.ID, archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE bot_versions SET status = 'active' WHERE id = $1`, v.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE game_bots SET active_version_id = $2 WHERE id = $1`, m.ID, v.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return u.ID, m.ID
}

func insertOpenMatch(t *testing.T, d *dbtest.DB) string {
	t.Helper()
	id := idgen.New("match")
	if err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO matches (id, game, kind, status, seed, map, ticks) VALUES ($1, 'tanks', 'ladder', 'queued', 1, 'arena', 100)`, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// insertMatchWithReplay inserts a bare finished match (no players) with a replay row, for PruneReplays.
func insertMatchWithReplay(t *testing.T, d *dbtest.DB, kind string, featured bool, finishedAt time.Time) string {
	t.Helper()
	id := idgen.New("match")
	if err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO matches (id, game, kind, status, seed, map, ticks, featured, created_at, started_at, finished_at)
			VALUES ($1, 'tanks', $2, 'finished', 1, 'arena', 100, $3, $4, $4, $4)`, id, kind, featured, finishedAt); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO match_replays (match_id, data) VALUES ($1, $2)`, id, []byte("fake-gzip-data"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// insertLadderMatch inserts a finished ladder match between the house hunter and sniper bots, with fixed
// mu_after values so RefreshBroadcast's "best combined rating" pick is deterministic.
func insertLadderMatch(t *testing.T, d *dbtest.DB, finishedAt time.Time, ticks int, muAfterHunter, muAfterSniper float64) string {
	t.Helper()
	id := idgen.New("match")
	if err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO matches (id, game, kind, status, seed, map, ticks, created_at, started_at, finished_at)
			VALUES ($1, 'tanks', 'ladder', 'finished', 1, 'arena', $2, $3, $3, $3)`, id, ticks, finishedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO match_players (match_id, slot, bot_id, version_id, place, kills, mu_before, sigma_before, mu_after, sigma_after)
			VALUES ($1, 0, 'bot_house_hunter', 'bv_house_hunter', 1, 5, 25, 8.333, $2, 1)`, id, muAfterHunter); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO match_players (match_id, slot, bot_id, version_id, place, kills, mu_before, sigma_before, mu_after, sigma_after)
			VALUES ($1, 1, 'bot_house_sniper', 'bv_house_sniper', 2, 1, 25, 8.333, $2, 5)`, id, muAfterSniper)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestScheduleHouseOnly(t *testing.T) {
	f := setupMatch(t)
	ctx := context.Background()

	id, err := f.svc.ScheduleTick(ctx, 5, 0)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if id == "" {
		t.Fatal("expected a house-only match")
	}
	mv, err := f.svc.Match(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if mv.Kind != "ladder" || len(mv.Players) != 2 {
		t.Fatalf("unexpected match: %+v", mv)
	}
	names := map[string]bool{}
	for _, p := range mv.Players {
		names[p.Name] = true
		if !p.House {
			t.Fatalf("expected every player to be a house bot, got %+v", p)
		}
	}
	if !names["hunter"] || !names["sniper"] {
		t.Fatalf("expected hunter and sniper, got %+v", mv.Players)
	}

	id2, err := f.svc.ScheduleTick(ctx, 5, 0)
	if err != nil {
		t.Fatalf("schedule 2: %v", err)
	}
	if id2 != "" {
		t.Fatalf("expected no second house-only match within 2 minutes, got %s", id2)
	}
}

func TestScheduleRespectsConcurrency(t *testing.T) {
	f := setupMatch(t)
	ctx := context.Background()

	insertOpenMatch(t, f.d)

	id, err := f.svc.ScheduleTick(ctx, 1, 0)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if id != "" {
		t.Fatalf("expected no match at concurrency 1 with an open match already queued, got %s", id)
	}
}

func TestScheduleFillsWithHouse(t *testing.T) {
	f := setupMatch(t)
	ctx := context.Background()

	_, botID := newUserBotWithArchive(t, f, "ace@example.com", "Ace", pythonStarterArchive(t))

	id, err := f.svc.ScheduleTick(ctx, 5, 0)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if id == "" {
		t.Fatal("expected a match")
	}
	mv, err := f.svc.Match(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(mv.Players) != 3 {
		t.Fatalf("expected 3 players (the user bot plus two house bots), got %d: %+v", len(mv.Players), mv.Players)
	}
	found := false
	houseCount := 0
	for _, p := range mv.Players {
		if p.BotID == botID {
			found = true
		}
		if p.House {
			houseCount++
		}
	}
	if !found {
		t.Fatalf("expected the user bot %s in the match: %+v", botID, mv.Players)
	}
	if houseCount != 2 {
		t.Fatalf("expected 2 house bots, got %d: %+v", houseCount, mv.Players)
	}
}

func TestRunMatchUpdatesRatings(t *testing.T) {
	requirePython3(t)
	f := setupMatch(t)
	ctx := context.Background()

	_, botID := newUserBotWithArchive(t, f, "idle-owner@example.com", "IdleBot", archiveWithFile(t, "bot.py", idlePy))

	matchID, err := f.svc.ScheduleTick(ctx, 5, 0)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if matchID == "" {
		t.Fatal("expected a scheduled match")
	}

	if err := f.svc.RunMatch(ctx, matchID); err != nil {
		t.Fatalf("RunMatch: %v", err)
	}

	mv, err := f.svc.Match(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if mv.Status != "finished" {
		t.Fatalf("status = %q, want finished", mv.Status)
	}
	if len(mv.Players) != 3 {
		t.Fatalf("expected 3 players, got %d", len(mv.Players))
	}

	sawBotID := false
	var winner *games.MatchPlayerView
	for i := range mv.Players {
		p := &mv.Players[i]
		if p.BotID == botID {
			sawBotID = true
		}
		if p.Place == nil {
			t.Fatalf("player %d has no place", p.Slot)
		}
		if p.RatingBefore == nil || p.RatingAfter == nil {
			t.Fatalf("player %d missing rating before/after: %+v", p.Slot, p)
		}
		if *p.Place == 1 {
			winner = p
		}
	}
	if !sawBotID {
		t.Fatalf("expected the user bot %s among the players: %+v", botID, mv.Players)
	}
	if winner == nil {
		t.Fatal("no place-1 player")
	}
	if *winner.RatingAfter <= *winner.RatingBefore {
		t.Fatalf("winner's rating did not increase: before=%d after=%d", *winner.RatingBefore, *winner.RatingAfter)
	}

	var matches, wins int
	if err := f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT matches, wins FROM game_bots WHERE id = $1`, winner.BotID).Scan(&matches, &wins)
	}); err != nil {
		t.Fatal(err)
	}
	if matches != 1 || wins != 1 {
		t.Fatalf("winner matches=%d wins=%d, want 1,1", matches, wins)
	}

	data, err := f.svc.Replay(ctx, matchID)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	replay, err := tanks.DecodeReplay(data)
	if err != nil {
		t.Fatalf("decode replay: %v", err)
	}
	if len(replay.Players) != 3 {
		t.Fatalf("expected 3 replay players, got %d", len(replay.Players))
	}
	for _, rp := range replay.Players {
		if rp.BotID == "" {
			t.Fatalf("replay player missing BotID: %+v", rp)
		}
	}
}

func TestRunMatchRerunAfterCrash(t *testing.T) {
	f := setupMatch(t)
	ctx := context.Background()

	matchID, err := f.svc.ScheduleTick(ctx, 5, 0)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if matchID == "" {
		t.Fatal("expected a scheduled match")
	}

	// Simulate a server restart mid-match: the match is stuck in running with no live worker behind it.
	if err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE matches SET status = 'running', started_at = now() WHERE id = $1`, matchID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := f.svc.RunMatch(ctx, matchID); err != nil {
		t.Fatalf("RunMatch (crash recovery): %v", err)
	}

	mv, err := f.svc.Match(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if mv.Status != "finished" {
		t.Fatalf("status = %q, want finished", mv.Status)
	}

	type snapshot struct{ mu, sigma float64 }
	before := map[string]snapshot{}
	for _, p := range mv.Players {
		_, mu, sigma := botRow(t, f.d, p.BotID)
		before[p.BotID] = snapshot{mu, sigma}
	}

	if err := f.svc.RunMatch(ctx, matchID); err != nil {
		t.Fatalf("RunMatch (already finished): %v", err)
	}

	for botID, want := range before {
		_, mu, sigma := botRow(t, f.d, botID)
		if mu != want.mu || sigma != want.sigma {
			t.Fatalf("bot %s rating changed on rerun of a finished match: before=%+v after=(%v,%v)", botID, want, mu, sigma)
		}
	}
}

func TestSweepStuck(t *testing.T) {
	f := setupMatch(t)
	ctx := context.Background()

	matchID, err := f.svc.ScheduleTick(ctx, 5, 0)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if matchID == "" {
		t.Fatal("expected a scheduled match")
	}
	mv, err := f.svc.Match(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}

	type snapshot struct{ mu, sigma float64 }
	before := map[string]snapshot{}
	for _, p := range mv.Players {
		_, mu, sigma := botRow(t, f.d, p.BotID)
		before[p.BotID] = snapshot{mu, sigma}
	}

	if err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE matches SET status = 'running', started_at = now() - interval '11 minutes' WHERE id = $1`, matchID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	n, err := f.svc.SweepStuck(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 1 {
		t.Fatalf("swept %d matches, want 1", n)
	}

	mv2, err := f.svc.Match(ctx, matchID)
	if err != nil {
		t.Fatal(err)
	}
	if mv2.Status != "infra_error" {
		t.Fatalf("status = %q, want infra_error", mv2.Status)
	}

	for botID, want := range before {
		_, mu, sigma := botRow(t, f.d, botID)
		if mu != want.mu || sigma != want.sigma {
			t.Fatalf("bot %s rating changed by the sweep: before=%+v after=(%v,%v)", botID, want, mu, sigma)
		}
	}
}

func TestPruneReplays(t *testing.T) {
	f := setupMatch(t)
	ctx := context.Background()
	old := time.Now().Add(-4 * 24 * time.Hour)

	plainID := insertMatchWithReplay(t, f.d, "ladder", false, old)
	featuredID := insertMatchWithReplay(t, f.d, "ladder", true, old)
	checkID := insertMatchWithReplay(t, f.d, "check", false, old)

	n, err := f.svc.PruneReplays(ctx)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("pruned %d replays, want 1", n)
	}

	mv, err := f.svc.Match(ctx, plainID)
	if err != nil {
		t.Fatal(err)
	}
	if mv.HasReplay {
		t.Fatal("expected the plain old match's replay to be pruned")
	}

	for _, id := range []string{featuredID, checkID} {
		mv, err := f.svc.Match(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !mv.HasReplay {
			t.Fatalf("expected %s's replay to survive pruning", id)
		}
	}
}

func TestBroadcastPicksBestAndRepeats(t *testing.T) {
	f := setupMatch(t)
	ctx := context.Background()

	now := time.Now()
	weakID := insertLadderMatch(t, f.d, now.Add(-time.Minute), 100, 26, 25)
	strongID := insertLadderMatch(t, f.d, now.Add(-time.Minute), 100, 60, 25)

	if err := f.svc.RefreshBroadcast(ctx); err != nil {
		t.Fatalf("refresh 1: %v", err)
	}
	live, err := f.svc.Live(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if live.MatchID == nil || *live.MatchID != strongID {
		t.Fatalf("expected the stronger match %s to be picked, got %v", strongID, live.MatchID)
	}
	mv, err := f.svc.Match(ctx, strongID)
	if err != nil {
		t.Fatal(err)
	}
	if !mv.Featured {
		t.Fatal("expected the picked match to be marked featured")
	}

	// Still airing (starts in 3s, lasts ticks*100ms): a refresh right away changes nothing.
	if err := f.svc.RefreshBroadcast(ctx); err != nil {
		t.Fatalf("refresh 2: %v", err)
	}
	live2, err := f.svc.Live(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if live2.MatchID == nil || *live2.MatchID != strongID || !live2.StartsAt.Equal(*live.StartsAt) {
		t.Fatalf("expected no change while still airing: %+v vs %+v", live, live2)
	}

	// End it: the remaining candidate (weakID) gets picked next.
	if err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE tanks_broadcasts SET starts_at = now() - interval '1 hour', duration_ms = 1000
			WHERE id = (SELECT id FROM tanks_broadcasts ORDER BY starts_at DESC LIMIT 1)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.RefreshBroadcast(ctx); err != nil {
		t.Fatalf("refresh 3: %v", err)
	}
	live3, err := f.svc.Live(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if live3.MatchID == nil || *live3.MatchID != weakID {
		t.Fatalf("expected the remaining candidate %s to be picked, got %v", weakID, live3.MatchID)
	}

	// End that one too: with no fresh candidate left, it repeats the last one shown.
	if err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE tanks_broadcasts SET starts_at = now() - interval '1 hour', duration_ms = 1000
			WHERE id = (SELECT id FROM tanks_broadcasts ORDER BY starts_at DESC LIMIT 1)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.RefreshBroadcast(ctx); err != nil {
		t.Fatalf("refresh 4: %v", err)
	}
	live4, err := f.svc.Live(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if live4.MatchID == nil || *live4.MatchID != weakID {
		t.Fatalf("expected a repeat of %s, got %v", weakID, live4.MatchID)
	}
}

func TestLeaderboardOrderAndIdleHidden(t *testing.T) {
	f := setupMatch(t)
	ctx := context.Background()

	entries, err := f.svc.Leaderboard(ctx, 50)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	for _, e := range entries {
		if e.BotID == "bot_house_idle" {
			t.Fatal("the idle house bot must not appear on the leaderboard")
		}
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least the hunter and sniper house bots, got %d", len(entries))
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Rating < entries[i].Rating {
			t.Fatalf("leaderboard is not sorted by descending rating: %+v", entries)
		}
		if entries[i].Rank != i+1 {
			t.Fatalf("rank at position %d = %d, want %d", i, entries[i].Rank, i+1)
		}
	}

	if err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE game_bots SET mu = 100 WHERE id = 'bot_house_sniper'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	entries2, err := f.svc.Leaderboard(ctx, 50)
	if err != nil {
		t.Fatalf("leaderboard 2: %v", err)
	}
	if entries2[0].BotID != "bot_house_sniper" || entries2[0].Rank != 1 {
		t.Fatalf("expected sniper to rank first after the boost, got %+v", entries2[0])
	}
}

func TestMatchLogOnlyOwnSlot(t *testing.T) {
	f := setupMatch(t)
	ctx := context.Background()

	userID, botID := newUserBotWithArchive(t, f, "logowner@example.com", "LogBot", pythonStarterArchive(t))

	matchID, err := f.svc.ScheduleTick(ctx, 5, 0)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if matchID == "" {
		t.Fatal("expected a scheduled match")
	}

	if err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE match_players SET stderr_tail = 'boom' WHERE match_id = $1 AND bot_id = $2`, matchID, botID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	logEntry, err := f.svc.MatchLog(ctx, userID, matchID)
	if err != nil {
		t.Fatalf("match log: %v", err)
	}
	if logEntry.Stderr != "boom" {
		t.Fatalf("stderr = %q, want boom", logEntry.Stderr)
	}
	if logEntry.MatchID != matchID {
		t.Fatalf("match id = %q, want %q", logEntry.MatchID, matchID)
	}

	other, _, err := f.users.SignIn(ctx, identity.Identity{Provider: "dev", Subject: "other@example.com", Email: "other@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.MatchLog(ctx, other.ID, matchID)
	problem(t, err, 404, "not_found")
}
