package games_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/games"
	"tolerance/internal/games/match"
	"tolerance/internal/games/tanks"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
)

// setupAgentRun is setupMatch (qualify_integration_test.go) plus an online agent for the caller, ready to
// drive proofs.CreateWithRepo / Claim / SubmitResult the way the real connector would.
func setupAgentRun(t *testing.T) (fixture, string) {
	t.Helper()
	f := setupMatch(t)
	ctx := context.Background()
	a, err := f.agents.Create(ctx, f.userID, agents.CreateInput{Name: "runner-agent"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := f.agents.Heartbeat(ctx, a.ID, "0.1", "h"); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	return f, a.ID
}

// untarFiles reads a tar.gz (as produced by proofs.TarFiles/TarDir) into a path -> contents map.
func untarFiles(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		out[h.Name] = body
	}
	return out
}

// makeDiff builds a unified diff (in the "--- a/<path>\n+++ b/<path>\n<hunks>" shape the proofs worker's
// applyDiff accepts) that turns oldContent into newContent at relPath, using the system `diff` tool so the
// hunk bodies don't have to be hand-maintained.
func makeDiff(t *testing.T, relPath, oldContent, newContent string) string {
	t.Helper()
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old")
	newPath := filepath.Join(dir, "new")
	if err := os.WriteFile(oldPath, []byte(oldContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte(newContent), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("diff", "-u", oldPath, newPath).Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("diff: %v", err)
		}
	}
	_, body, ok := cutTwoLines(string(out))
	if !ok {
		t.Fatalf("diff produced no hunks between old and new content for %s", relPath)
	}
	return fmt.Sprintf("--- a/%s\n+++ b/%s\n%s", relPath, relPath, body)
}

// deletionDiff builds a unified diff that deletes relPath (whose current contents are content).
func deletionDiff(t *testing.T, relPath, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("diff", "-u", path, os.DevNull).Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("diff: %v", err)
		}
	}
	_, body, ok := cutTwoLines(string(out))
	if !ok {
		t.Fatalf("diff produced no hunks deleting %s", relPath)
	}
	return fmt.Sprintf("--- a/%s\n+++ /dev/null\n%s", relPath, body)
}

// cutTwoLines drops the first two lines of s (the "--- old" / "+++ new" header diff(1) prints, which the
// caller replaces with its own a/ b/ headers) and returns the rest.
func cutTwoLines(s string) (first, rest string, ok bool) {
	i := strings.Index(s, "\n")
	if i < 0 {
		return "", "", false
	}
	j := strings.Index(s[i+1:], "\n")
	if j < 0 {
		return "", "", false
	}
	return s[:i+1+j+1], s[i+1+j+1:], true
}

func TestStartAgentRunBuildsRepo(t *testing.T) {
	f, agentID := setupAgentRun(t)
	ctx := context.Background()

	p, err := f.svc.StartAgentRun(ctx, f.userID)
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	if p.Kind != proofs.KindGameBot {
		t.Errorf("kind = %q, want %q", p.Kind, proofs.KindGameBot)
	}
	if p.TaskSlug != "tanks-bot" {
		t.Errorf("task_slug = %q, want tanks-bot", p.TaskSlug)
	}

	claimed, task, err := f.proofs.Claim(ctx, agentID)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	if claimed.ID != p.ID {
		t.Fatalf("claimed %s, want %s", claimed.ID, p.ID)
	}
	files := untarFiles(t, task.RepoTar)
	for _, name := range []string{"bot.json", "bot.py", "tanks.py", "GAME.md", "RESULTS.md"} {
		if _, ok := files[name]; !ok {
			t.Errorf("repo missing %s (have %v)", name, keysOf(files))
		}
	}
	if !strings.Contains(string(files["RESULTS.md"]), "No matches yet") {
		t.Errorf("RESULTS.md for a bot with no history should say so, got:\n%s", files["RESULTS.md"])
	}

	botID := botIDFor(t, f.d, f.userID)
	var name string
	if err := f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT name FROM game_bots WHERE id = $1`, botID).Scan(&name)
	}); err != nil {
		t.Fatal(err)
	}
	if name != "runner-agent" {
		t.Fatalf("bot name = %q, want the agent's name", name)
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestStartAgentRunUsesActiveVersion(t *testing.T) {
	f, agentID := setupAgentRun(t)
	ctx := context.Background()

	const marker = "# marker-b3f1\n"
	archive := archiveWithFile(t, "bot.py", marker)
	v, err := f.svc.UploadVersion(ctx, f.userID, archive)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	botID := botIDFor(t, f.d, f.userID)
	// Promote the version directly rather than through Qualify (which would run a real match against a
	// non-functional bot.py) - this test is about StartAgentRun reading whatever active_version_id points
	// to, not about qualification itself.
	if err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE bot_versions SET status = 'active' WHERE id = $1`, v.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE game_bots SET active_version_id = $2 WHERE id = $1`, botID, v.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	p, err := f.svc.StartAgentRun(ctx, f.userID)
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	claimed, task, err := f.proofs.Claim(ctx, agentID)
	if err != nil || claimed == nil || claimed.ID != p.ID {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	files := untarFiles(t, task.RepoTar)
	if string(files["bot.py"]) != marker {
		t.Fatalf("bot.py = %q, want the active version's content %q", files["bot.py"], marker)
	}
}

func TestStartAgentRunOffline(t *testing.T) {
	f := setupMatch(t)
	ctx := context.Background()
	if _, err := f.agents.Create(ctx, f.userID, agents.CreateInput{Name: "runner-agent"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	_, err := f.svc.StartAgentRun(ctx, f.userID)
	problem(t, err, 409, "agent_offline")
}

// starterBotPy is the unmodified python starter kit's bot.py, read fresh from tanks.Starter so these
// tests never drift from the real starter kit.
func starterBotPy(t *testing.T) string {
	t.Helper()
	files, err := tanks.Starter("python")
	if err != nil {
		t.Fatal(err)
	}
	return string(files["bot.py"])
}

func TestGameBotProofEndToEnd(t *testing.T) {
	requirePython3(t)
	f, agentID := setupAgentRun(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	p, err := f.svc.StartAgentRun(ctx, f.userID)
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	claimed, _, err := f.proofs.Claim(ctx, agentID)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %+v", err, claimed)
	}

	old := starterBotPy(t)
	newContent := strings.Replace(old, "0.08", "0.05", 1)
	if newContent == old {
		t.Fatal("expected to find the constant 0.08 in the starter bot.py")
	}
	diff := makeDiff(t, "bot.py", old, newContent)
	if err := f.proofs.SubmitResult(ctx, agentID, claimed.ID, proofs.ResultInput{Diff: diff}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	w := proofs.NewWorker(f.d.AppPool, sandbox.PassAll{}, t.TempDir(), log)
	w.SetGameBotJudge(f.svc)
	if err := w.RunProof(ctx, p.ID); err != nil {
		t.Fatalf("RunProof: %v", err)
	}

	got, err := f.proofs.Get(ctx, f.userID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != proofs.StatusPassed {
		out := ""
		if got.SandboxResult != nil {
			out = got.SandboxResult.Output
		}
		t.Fatalf("status = %q reason = %q output=%s", got.Status, got.FailureReason, out)
	}
	if got.SandboxResult == nil || len(got.SandboxResult.Tests) != 4 {
		t.Fatalf("expected 4 checks, got %+v", got.SandboxResult)
	}

	botID := botIDFor(t, f.d, f.userID)
	activeVersionID, _, _ := botRow(t, f.d, botID)
	if activeVersionID == nil {
		t.Fatal("expected the bot to have a new active version")
	}
	var source string
	var proofID *string
	if err := f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT source, proof_id FROM bot_versions WHERE id = $1`, *activeVersionID).Scan(&source, &proofID)
	}); err != nil {
		t.Fatal(err)
	}
	if source != "agent" {
		t.Errorf("source = %q, want agent", source)
	}
	if proofID == nil || *proofID != p.ID {
		t.Errorf("proof_id = %v, want %s", proofID, p.ID)
	}
}

func TestGameBotProofBrokenBot(t *testing.T) {
	requirePython3(t)
	f, agentID := setupAgentRun(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	p, err := f.svc.StartAgentRun(ctx, f.userID)
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	claimed, _, err := f.proofs.Claim(ctx, agentID)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %+v", err, claimed)
	}

	diff := makeDiff(t, "bot.py", starterBotPy(t), "raise SystemExit(1)\n")
	if err := f.proofs.SubmitResult(ctx, agentID, claimed.ID, proofs.ResultInput{Diff: diff}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	w := proofs.NewWorker(f.d.AppPool, sandbox.PassAll{}, t.TempDir(), log)
	w.SetGameBotJudge(f.svc)
	if err := w.RunProof(ctx, p.ID); err != nil {
		t.Fatalf("RunProof: %v", err)
	}

	got, err := f.proofs.Get(ctx, f.userID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != proofs.StatusFailed || got.FailureReason != "bot_rejected" {
		t.Fatalf("%+v", got)
	}

	botID := botIDFor(t, f.d, f.userID)
	activeVersionID, _, _ := botRow(t, f.d, botID)
	if activeVersionID != nil {
		t.Fatalf("expected no active version for a bot that only ever submitted a crashing build, got %v", *activeVersionID)
	}
}

func TestGameBotProofDeletesManifest(t *testing.T) {
	requirePython3(t)
	f, agentID := setupAgentRun(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	p, err := f.svc.StartAgentRun(ctx, f.userID)
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	claimed, task, err := f.proofs.Claim(ctx, agentID)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	files := untarFiles(t, task.RepoTar)
	botJSON, ok := files["bot.json"]
	if !ok {
		t.Fatal("repo has no bot.json to delete")
	}

	diff := deletionDiff(t, "bot.json", string(botJSON))
	if err := f.proofs.SubmitResult(ctx, agentID, claimed.ID, proofs.ResultInput{Diff: diff}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	w := proofs.NewWorker(f.d.AppPool, sandbox.PassAll{}, t.TempDir(), log)
	w.SetGameBotJudge(f.svc)
	if err := w.RunProof(ctx, p.ID); err != nil {
		t.Fatalf("RunProof: %v", err)
	}

	got, err := f.proofs.Get(ctx, f.userID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != proofs.StatusFailed || got.FailureReason != "invalid_package" {
		t.Fatalf("%+v", got)
	}
	if got.SandboxResult == nil || len(got.SandboxResult.Tests) != 1 ||
		got.SandboxResult.Tests[0].Name != "package" || got.SandboxResult.Tests[0].Passed {
		t.Fatalf("tests = %+v", got.SandboxResult)
	}
	if got.SandboxResult.Output == "" {
		t.Fatal("expected an error message in Output")
	}
}

// setupAgentRunWithLauncher is setupAgentRun with a caller-supplied non-house launcher (wrapped in
// match.WithHouse), so a test can make the check match's own bot launch fail on demand.
func setupAgentRunWithLauncher(t *testing.T, l match.Launcher) (fixture, string) {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	if err := games.Sync(ctx, d.AdminPool); err != nil {
		t.Fatal(err)
	}
	ps := proofs.NewService(d.AppPool)
	as := agents.NewService(d.AppPool, ps)
	us := identity.NewService(d.AppPool, nil)
	svc := games.NewService(d.AppPool, ps, match.WithHouse(l), slog.Default(), games.Config{CheckTicks: 200, WorkDir: t.TempDir()})
	u, _, err := us.Signup(ctx, "o@example.com", "longenough1")
	if err != nil {
		t.Fatal(err)
	}
	a, err := as.Create(ctx, u.ID, agents.CreateInput{Name: "runner-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := as.Heartbeat(ctx, a.ID, "0.1", "h"); err != nil {
		t.Fatal(err)
	}
	return fixture{d: d, svc: svc, users: us, agents: as, proofs: ps, userID: u.ID}, a.ID
}

// flakyLauncher fails the first failLeft launches (of a non-house spec - WithHouse never forwards a house
// spec to it) and then behaves exactly like next, simulating a platform hiccup (docker down, ...) on an
// otherwise good run.
type flakyLauncher struct {
	mu       sync.Mutex
	failLeft int
	next     match.Launcher
}

func (l *flakyLauncher) Launch(ctx context.Context, s match.Spec) (match.Bot, error) {
	l.mu.Lock()
	fail := l.failLeft > 0
	if fail {
		l.failLeft--
	}
	l.mu.Unlock()
	if fail {
		return nil, fmt.Errorf("flakyLauncher: simulated launch failure")
	}
	return l.next.Launch(ctx, s)
}

// TestJudgeProofRetrySafeAfterQualifyFails is the regression test for the bug a review caught: JudgeProof
// used to insert a brand new bot_versions row on every call, so a run_proof job retry after a platform
// failure inside Qualify (the check match's launcher erroring, not the bot's own fault) left the first
// attempt's version stuck 'pending' forever and qualified a second, redundant one. createVersion now
// reuses whatever version this proof already produced (looked up by (bot_id, proof_id)) instead of
// inserting again, so a retry re-qualifies the *same* version - which Qualify itself already treats as
// idempotent.
func TestJudgeProofRetrySafeAfterQualifyFails(t *testing.T) {
	requirePython3(t)
	flaky := &flakyLauncher{failLeft: 1, next: match.ProcessLauncher{}}
	f, agentID := setupAgentRunWithLauncher(t, flaky)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	p, err := f.svc.StartAgentRun(ctx, f.userID)
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	claimed, _, err := f.proofs.Claim(ctx, agentID)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	old := starterBotPy(t)
	diff := makeDiff(t, "bot.py", old, strings.Replace(old, "0.08", "0.05", 1))
	if err := f.proofs.SubmitResult(ctx, agentID, claimed.ID, proofs.ResultInput{Diff: diff}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	w := proofs.NewWorker(f.d.AppPool, sandbox.PassAll{}, t.TempDir(), log)
	w.SetGameBotJudge(f.svc)

	// First attempt: Qualify's own check match fails to even launch the candidate bot - a platform
	// failure, not a verdict on the bot - so RunProof must return an error (the job would retry) and
	// leave the proof running_sandbox, not finished.
	if err := w.RunProof(ctx, p.ID); err == nil {
		t.Fatal("expected the first attempt to fail: the launcher is flaky")
	}
	got, err := f.proofs.Get(ctx, f.userID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != proofs.StatusRunningSandbox {
		t.Fatalf("proof should still be running_sandbox after a platform failure, got %+v", got)
	}

	botID := botIDFor(t, f.d, f.userID)
	countVersions := func() int {
		t.Helper()
		var n int
		if err := f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM bot_versions WHERE bot_id = $1 AND proof_id = $2`, botID, p.ID).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := countVersions(); n != 1 {
		t.Fatalf("expected exactly one bot_versions row after the first (failed) attempt, got %d", n)
	}

	// Retry: the job runner would re-claim and call RunProof again with the same proof id.
	if err := w.RunProof(ctx, p.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	got, err = f.proofs.Get(ctx, f.userID, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != proofs.StatusPassed {
		out := ""
		if got.SandboxResult != nil {
			out = got.SandboxResult.Output
		}
		t.Fatalf("expected the retry to pass, got status=%q reason=%q output=%s", got.Status, got.FailureReason, out)
	}
	if n := countVersions(); n != 1 {
		t.Fatalf("expected exactly one bot_versions row for this proof after the retry, got %d", n)
	}
}

// TestJudgeProofIdempotentAfterSuccess is JudgeProof's second idempotency requirement: called again after
// it already produced a resolved (active/rejected) verdict for a proof, it must not create another version
// or run another check match - Qualify on the same, already-resolved version id just hands back what it
// already stored, so the verdict is identical.
func TestJudgeProofIdempotentAfterSuccess(t *testing.T) {
	requirePython3(t)
	f, agentID := setupAgentRun(t)
	ctx := context.Background()

	p, err := f.svc.StartAgentRun(ctx, f.userID)
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}

	dir := t.TempDir()
	starter, err := tanks.Starter("python")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range starter {
		if name == "GAME.md" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	v1, err := f.svc.JudgeProof(ctx, p.ID, agentID, dir)
	if err != nil {
		t.Fatalf("first JudgeProof: %v", err)
	}
	if !v1.Passed {
		t.Fatalf("expected the unmodified starter kit to pass, got %+v", v1)
	}

	v2, err := f.svc.JudgeProof(ctx, p.ID, agentID, dir)
	if err != nil {
		t.Fatalf("second JudgeProof: %v", err)
	}
	if !reflect.DeepEqual(v1, v2) {
		t.Fatalf("second call returned a different verdict:\nfirst:  %+v\nsecond: %+v", v1, v2)
	}

	botID := botIDFor(t, f.d, f.userID)
	var count int
	if err := f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM bot_versions WHERE bot_id = $1 AND proof_id = $2`, botID, p.ID).Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one bot_versions row for this proof, got %d", count)
	}
}
