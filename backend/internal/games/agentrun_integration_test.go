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
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/games/tanks"
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
