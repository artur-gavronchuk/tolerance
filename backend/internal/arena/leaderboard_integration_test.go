package arena_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/arena"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/idgen"
)

func mustExec(t *testing.T, d *dbtest.DB, sql string, args ...any) {
	t.Helper()
	err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
	if err != nil {
		t.Fatalf("seed (%s): %v", sql, err)
	}
}

// seedRated creates an agent with a rating on the named skill. onCurrent = false
// means the rating was earned on a version the owner has since replaced, so the
// agent's current_version_id points past the version the rating names.
func seedRated(t *testing.T, d *dbtest.DB, name, skill string, rating, uncertainty int, onCurrent, public bool) {
	t.Helper()
	userID, agentID := idgen.New("user"), idgen.New("agent")
	mustExec(t, d, `INSERT INTO users (id, email) VALUES ($1, $1 || '@example.com')`, userID)
	mustExec(t, d, `INSERT INTO agents (id, owner_user_id, name, public) VALUES ($1, $2, $3, $4)`, agentID, userID, name, public)
	mustExec(t, d, `INSERT INTO skills (slug, title, language, image, run_cmd)
		VALUES ($1, $1, 'go', 'arena-skill-go:1', 'go test -json ./...') ON CONFLICT DO NOTHING`, skill)
	rated := idgen.New("ver")
	mustExec(t, d, `INSERT INTO agent_versions (id, agent_id, number, model, harness, config_digest)
		VALUES ($1, $2, 1, 'claude-opus-5', 'claude-code 2.1', $1)`, rated, agentID)
	current := rated
	if !onCurrent {
		current = idgen.New("ver")
		mustExec(t, d, `INSERT INTO agent_versions (id, agent_id, number, model, harness, config_digest)
			VALUES ($1, $2, 2, 'claude-sonnet-5-5', 'claude-code 2.1', $1)`, current, agentID)
	}
	mustExec(t, d, `UPDATE agents SET current_version_id = $2 WHERE id = $1`, agentID, current)
	mustExec(t, d, `INSERT INTO skill_ratings (agent_id, skill_slug, version_id, rating, uncertainty, runs, sum_targets)
		VALUES ($1, $2, $3, $4, $5, 1, $4)`, agentID, skill, rated, rating, uncertainty)
}

func names(rows []arena.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.AgentName
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLeaderboardOrdersByAccessThenConfirmed(t *testing.T) {
	d := dbtest.New(t)
	seedRated(t, d, "low", "go", 1600, 100, true, true)    // access 1500
	seedRated(t, d, "high", "go", 2000, 100, true, true)   // access 1900
	seedRated(t, d, "stale", "go", 2000, 100, false, true) // access 1900, not confirmed
	seedRated(t, d, "hidden", "go", 2400, 60, true, false) // opted out
	seedRated(t, d, "other", "python", 2400, 60, true, true)

	rows, err := arena.NewService(d.AppPool).Leaderboard(context.Background(), "go", 100)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	// On equal access the version-confirmed row comes first. An opted-out agent
	// and another skill's agent are absent.
	if want := []string{"high", "stale", "low"}; !equal(names(rows), want) {
		t.Fatalf("rows = %v, want %v", names(rows), want)
	}
	if rows[0].Rank != 1 || rows[1].Rank != 2 || rows[2].Rank != 3 {
		t.Errorf("ranks = %d, %d, %d, want 1, 2, 3", rows[0].Rank, rows[1].Rank, rows[2].Rank)
	}
	if rows[0].Access != 1900 || rows[0].Tier != "strong" {
		t.Errorf("first row = access %d tier %q, want 1900 strong", rows[0].Access, rows[0].Tier)
	}
	if !rows[0].OnCurrentVersion || rows[1].OnCurrentVersion {
		t.Error("on_current_version must be true for the confirmed row and false for the stale one")
	}
	if rows[1].VersionNumber != 1 || rows[1].Model != "claude-opus-5" {
		t.Errorf("a stale row names the version the rating was earned on, got v%d %q", rows[1].VersionNumber, rows[1].Model)
	}
	if rows[0].ScoredAt.Location().String() != "UTC" {
		t.Errorf("scored_at must be UTC, got %s", rows[0].ScoredAt.Location())
	}
}

func TestLeaderboardHidesBannedAgents(t *testing.T) {
	d := dbtest.New(t)
	seedRated(t, d, "fine", "go", 1800, 100, true, true)
	seedRated(t, d, "banned", "go", 2400, 60, true, true)
	mustExec(t, d, `UPDATE agents SET banned_at = now(), banned_reason = 'test' WHERE name = 'banned'`)

	rows, err := arena.NewService(d.AppPool).Leaderboard(context.Background(), "go", 100)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if want := []string{"fine"}; !equal(names(rows), want) {
		t.Fatalf("rows = %v, want %v", names(rows), want)
	}
}

func TestLeaderboardClampsToLimitAndIsEmptyNotNil(t *testing.T) {
	d := dbtest.New(t)
	for _, n := range []string{"a", "b", "c"} {
		seedRated(t, d, n, "go", 1800, 100, true, true)
	}
	s := arena.NewService(d.AppPool)
	rows, err := s.Leaderboard(context.Background(), "go", 2)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	empty, err := s.Leaderboard(context.Background(), "rust", 100)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if empty == nil {
		t.Fatal("an unknown skill must return an empty slice, not nil, so the JSON is [] and not null")
	}
}
