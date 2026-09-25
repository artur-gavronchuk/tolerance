package games_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/games"
	"tolerance/internal/games/match"
	"tolerance/internal/games/tanks"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/proofs"
)

// requirePython3 skips a test when python3 isn't on PATH, unless ARENA_TEST_REQUIRE_DOCKER=1 (CI always
// has it), in which case a missing interpreter fails the test rather than silently skipping it - the same
// convention internal/games/match's own tests use for requireInterpreter.
func requirePython3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		if os.Getenv("ARENA_TEST_REQUIRE_DOCKER") == "1" {
			t.Fatalf("python3 not found in PATH: %v", err)
		}
		t.Skipf("python3 not found in PATH: %v", err)
	}
}

// setupMatch is setup (bots_integration_test.go) with a real process launcher wired in, so bots actually
// run, and short check matches so the suite stays fast.
func setupMatch(t *testing.T) fixture {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	if err := games.Sync(ctx, d.AdminPool); err != nil {
		t.Fatal(err)
	}
	ps := proofs.NewService(d.AppPool)
	as := agents.NewService(d.AppPool, ps)
	us := identity.NewService(d.AppPool, nil)
	svc := games.NewService(d.AppPool, ps, match.WithHouse(match.ProcessLauncher{}), slog.Default(),
		games.Config{CheckTicks: 200, WorkDir: t.TempDir()})
	u, _, err := us.Signup(ctx, "o@example.com", "longenough1")
	if err != nil {
		t.Fatal(err)
	}
	return fixture{d: d, svc: svc, users: us, agents: as, proofs: ps, userID: u.ID}
}

// pythonStarterArchive is the unmodified python starter kit, tarred as a bot upload.
func pythonStarterArchive(t *testing.T) []byte {
	t.Helper()
	files, err := tanks.Starter("python")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := proofs.TarFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

// archiveWithFile is the python starter kit with one file's contents replaced.
func archiveWithFile(t *testing.T, path, content string) []byte {
	t.Helper()
	files, err := tanks.Starter("python")
	if err != nil {
		t.Fatal(err)
	}
	files[path] = []byte(content)
	archive, err := proofs.TarFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

const noisyHunterPy = `import sys
import tanks


class NoisyHunter:
    def start(self, msg):
        self.me_id = msg["you"]

    def decide(self, msg):
        print("debug")
        me = next(t for t in msg["tanks"] if t["id"] == self.me_id)
        if not me["alive"]:
            return {"move": 0, "turn": 0, "turret": 0, "fire": False}
        target = tanks.nearest_enemy(me, msg["tanks"])
        if target is None:
            return {"move": 0, "turn": 0, "turret": 0, "fire": False}
        target_angle = tanks.angle_to(me["x"], me["y"], target["x"], target["y"])
        hull_diff = tanks.angle_diff(target_angle, me["hull"])
        turn = max(-1.0, min(1.0, 3 * hull_diff))
        dist = tanks.distance(me["x"], me["y"], target["x"], target["y"])
        move = 1.0 if dist > 6 else 0.0
        turret_diff = tanks.angle_diff(target_angle, me["turret"])
        turret = max(-1.0, min(1.0, 4 * turret_diff))
        fire = abs(turret_diff) < 0.08
        return {"move": move, "turn": turn, "turret": turret, "fire": fire}


if __name__ == "__main__":
    tanks.run(NoisyHunter())
`

// qualifyUntilPassed uploads archive as a fresh pending version and qualifies it, retrying with another
// fresh upload of the same archive if the version doesn't pass, up to a bounded number of attempts. The
// check match's third slot is a full-strength house hunter (internal/games/tanks/house), so even a
// perfectly competent bot doesn't win that three-way fight on every random seed - only in the clear
// majority of them - so a single attempt would make this test flaky. 20 attempts against a >=60%
// per-attempt win rate fail on the order of 1e-4 of runs.
func qualifyUntilPassed(t *testing.T, f fixture, archive []byte) games.VersionView {
	t.Helper()
	ctx := context.Background()
	for attempt := 1; attempt <= 20; attempt++ {
		v, err := f.svc.UploadVersion(ctx, f.userID, archive)
		if err != nil {
			t.Fatalf("upload (attempt %d): %v", attempt, err)
		}
		passed, checks, err := f.svc.Qualify(ctx, v.ID)
		if err != nil {
			t.Fatalf("qualify (attempt %d): %v", attempt, err)
		}
		if passed {
			return v
		}
		t.Logf("attempt %d did not pass, retrying with a fresh check match: %+v", attempt, checks)
	}
	t.Fatal("did not qualify after 20 attempts")
	return games.VersionView{}
}

func botIDFor(t *testing.T, d *dbtest.DB, userID string) string {
	t.Helper()
	var id string
	err := d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM game_bots WHERE owner_user_id = $1`, userID).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func setBotSigma(t *testing.T, d *dbtest.DB, botID string, sigma float64) {
	t.Helper()
	if err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE game_bots SET sigma = $2 WHERE id = $1`, botID, sigma)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func botRow(t *testing.T, d *dbtest.DB, botID string) (activeVersionID *string, mu, sigma float64) {
	t.Helper()
	if err := d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT active_version_id, mu, sigma FROM game_bots WHERE id = $1`, botID).
			Scan(&activeVersionID, &mu, &sigma)
	}); err != nil {
		t.Fatal(err)
	}
	return activeVersionID, mu, sigma
}

func versionRow(t *testing.T, d *dbtest.DB, versionID string) (status, checkMatchID string) {
	t.Helper()
	var cm *string
	if err := d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT status, check_match_id FROM bot_versions WHERE id = $1`, versionID).Scan(&status, &cm)
	}); err != nil {
		t.Fatal(err)
	}
	if cm != nil {
		checkMatchID = *cm
	}
	return status, checkMatchID
}

func hasReplay(t *testing.T, d *dbtest.DB, matchID string) bool {
	t.Helper()
	var n int
	if err := d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM match_replays WHERE match_id = $1`, matchID).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestQualifyStarterActivates(t *testing.T) {
	requirePython3(t)
	f := setupMatch(t)

	v := qualifyUntilPassed(t, f, pythonStarterArchive(t))

	status, checkMatchID := versionRow(t, f.d, v.ID)
	if status != "active" {
		t.Fatalf("version status = %q, want active", status)
	}
	if checkMatchID == "" {
		t.Fatal("expected a check_match_id")
	}
	if !hasReplay(t, f.d, checkMatchID) {
		t.Fatal("expected the check match to have a replay")
	}

	botID := botIDFor(t, f.d, f.userID)
	activeVersionID, _, _ := botRow(t, f.d, botID)
	if activeVersionID == nil || *activeVersionID != v.ID {
		t.Fatalf("bot active_version_id = %v, want %s", activeVersionID, v.ID)
	}

	var kind string
	if err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT kind FROM matches WHERE id = $1`, checkMatchID).Scan(&kind)
	}); err != nil {
		t.Fatal(err)
	}
	if kind != "check" {
		t.Fatalf("check match kind = %q, want check", kind)
	}
}

func TestQualifyCrashingBotRejected(t *testing.T) {
	requirePython3(t)
	f := setupMatch(t)
	ctx := context.Background()

	v1 := qualifyUntilPassed(t, f, pythonStarterArchive(t))
	botID := botIDFor(t, f.d, f.userID)

	crashArchive := archiveWithFile(t, "bot.py", "raise SystemExit(1)\n")
	v2, err := f.svc.UploadVersion(ctx, f.userID, crashArchive)
	if err != nil {
		t.Fatalf("upload v2: %v", err)
	}

	passed, checks, err := f.svc.Qualify(ctx, v2.ID)
	if err != nil {
		t.Fatalf("qualify v2: %v", err)
	}
	if passed {
		t.Fatalf("expected the crashing bot to fail, checks=%+v", checks)
	}
	if len(checks) != 4 {
		t.Fatalf("expected 4 checks, got %d: %+v", len(checks), checks)
	}
	if !checks[0].Passed {
		t.Errorf("package check should still pass: %+v", checks[0])
	}
	if checks[1].Name != "starts" || checks[1].Passed {
		t.Errorf("starts check should fail: %+v", checks[1])
	}
	for _, c := range checks[2:] {
		if c.Passed || c.Detail != "skipped" {
			t.Errorf("expected %s to be skipped, got %+v", c.Name, c)
		}
	}

	status, _ := versionRow(t, f.d, v2.ID)
	if status != "rejected" {
		t.Fatalf("version status = %q, want rejected", status)
	}

	activeVersionID, _, _ := botRow(t, f.d, botID)
	if activeVersionID == nil || *activeVersionID != v1.ID {
		t.Fatalf("active_version_id = %v, want the previous version %s to stay active", activeVersionID, v1.ID)
	}
}

func TestQualifyNoisyBotHint(t *testing.T) {
	requirePython3(t)
	f := setupMatch(t)

	v := qualifyUntilPassed(t, f, archiveWithFile(t, "bot.py", noisyHunterPy))

	status, _ := versionRow(t, f.d, v.ID)
	if status != "active" {
		t.Fatalf("version status = %q, want active", status)
	}

	var raw []byte
	if err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT checks FROM bot_versions WHERE id = $1`, v.ID).Scan(&raw)
	}); err != nil {
		t.Fatal(err)
	}
	var checks []games.Check
	if err := json.Unmarshal(raw, &checks); err != nil {
		t.Fatal(err)
	}
	var stable *games.Check
	for i := range checks {
		if checks[i].Name == "stable" {
			stable = &checks[i]
		}
	}
	if stable == nil {
		t.Fatal("no stable check")
	}
	if !strings.Contains(stable.Detail, "stderr") {
		t.Errorf("stable detail should hint at stderr for noisy stdout, got %q", stable.Detail)
	}
}

func TestQualifyRefreshesSigma(t *testing.T) {
	requirePython3(t)
	f := setupMatch(t)
	archive := pythonStarterArchive(t)

	v1 := qualifyUntilPassed(t, f, archive)
	botID := botIDFor(t, f.d, f.userID)
	_, muBefore, _ := botRow(t, f.d, botID)
	setBotSigma(t, f.d, botID, 2)

	v2 := qualifyUntilPassed(t, f, archive)
	if v2.ID == v1.ID {
		t.Fatal("expected a second, distinct version")
	}

	_, muAfter, sigmaAfter := botRow(t, f.d, botID)
	if sigmaAfter != 5.0 {
		t.Fatalf("sigma = %v, want 5.0", sigmaAfter)
	}
	if muAfter != muBefore {
		t.Fatalf("mu changed: before=%v after=%v, want unchanged", muBefore, muAfter)
	}
}

func TestQualifyIdempotent(t *testing.T) {
	requirePython3(t)
	f := setupMatch(t)
	ctx := context.Background()

	v := qualifyUntilPassed(t, f, pythonStarterArchive(t))
	status1, checkMatchID1 := versionRow(t, f.d, v.ID)
	if status1 != "active" {
		t.Fatalf("version status = %q, want active", status1)
	}

	var matchCountBefore int
	if err := f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM matches WHERE kind = 'check'`).Scan(&matchCountBefore)
	}); err != nil {
		t.Fatal(err)
	}

	passed2, checks2, err := f.svc.Qualify(ctx, v.ID)
	if err != nil {
		t.Fatalf("qualify 2: %v", err)
	}
	if !passed2 {
		t.Fatalf("expected the second call to report the already-active result, got passed=%v checks=%+v", passed2, checks2)
	}
	if len(checks2) != 4 {
		t.Fatalf("expected the stored 4 checks back, got %d: %+v", len(checks2), checks2)
	}

	status2, checkMatchID2 := versionRow(t, f.d, v.ID)
	if status2 != "active" {
		t.Fatalf("version status = %q, want active", status2)
	}
	if checkMatchID1 != checkMatchID2 {
		t.Fatalf("check_match_id changed: %s -> %s", checkMatchID1, checkMatchID2)
	}

	var matchCountAfter int
	if err := f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM matches WHERE kind = 'check'`).Scan(&matchCountAfter)
	}); err != nil {
		t.Fatal(err)
	}
	if matchCountBefore != matchCountAfter {
		t.Fatalf("expected no new check match, before=%d after=%d", matchCountBefore, matchCountAfter)
	}
}
