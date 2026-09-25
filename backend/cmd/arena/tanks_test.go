package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"tolerance/internal/games/tanks"
)

// requireInterpreter skips a test when bin isn't on PATH, unless ARENA_TEST_REQUIRE_DOCKER=1 (CI sets that
// flag and always has both python3 and node), in which case a missing interpreter fails the test. Mirrors
// internal/games/match's helper of the same name.
func requireInterpreter(t *testing.T, bin string) {
	t.Helper()
	if _, err := exec.LookPath(bin); err != nil {
		if os.Getenv("ARENA_TEST_REQUIRE_DOCKER") == "1" {
			t.Fatalf("%s not found in PATH: %v", bin, err)
		}
		t.Skipf("%s not found in PATH: %v", bin, err)
	}
}

func TestTanksNewWritesStarter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mybot")
	var stdout bytes.Buffer
	if err := runTanks([]string{"new", dir}, &stdout, &stdout); err != nil {
		t.Fatalf("tanks new: %v", err)
	}
	for _, want := range []string{"bot.json", "bot.py", "tanks.py", "GAME.md"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
	if !strings.Contains(stdout.String(), "arena tanks play") {
		t.Errorf("expected next-steps hint in output, got %q", stdout.String())
	}
}

func TestTanksNewRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "already-here"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := runTanks([]string{"new", dir}, &out, &out)
	if err == nil {
		t.Fatal("expected an error for a non-empty directory")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "bot.json")); statErr == nil {
		t.Fatal("must not have written into a non-empty directory")
	}
}

func TestTanksPlayHouseOnly(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "replay.json")
	var stdout bytes.Buffer
	err := runTanks([]string{"play", "house:hunter", "house:idle", "--ticks", "200", "--out", out}, &stdout, &stdout)
	if err != nil {
		t.Fatalf("tanks play: %v", err)
	}
	if !strings.Contains(stdout.String(), "hunter") {
		t.Errorf("expected hunter in output, got:\n%s", stdout.String())
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("replay file: %v", err)
	}
	var replay tanks.Replay
	if err := json.Unmarshal(data, &replay); err != nil {
		t.Fatalf("replay is not valid JSON: %v", err)
	}
	if len(replay.Players) != 2 {
		t.Errorf("replay has %d players, want 2", len(replay.Players))
	}
	if len(replay.Frames) == 0 {
		t.Error("replay has no frames")
	}
}

func TestTanksPlayStarter(t *testing.T) {
	requireInterpreter(t, "python3")

	botDir := filepath.Join(t.TempDir(), "mybot")
	var newOut bytes.Buffer
	if err := runTanks([]string{"new", botDir, "--lang", "python"}, &newOut, &newOut); err != nil {
		t.Fatalf("tanks new: %v", err)
	}

	out := filepath.Join(t.TempDir(), "replay.json")
	var playOut bytes.Buffer
	err := runTanks([]string{"play", botDir, "house:idle", "--ticks", "50", "--out", out}, &playOut, &playOut)
	if err != nil {
		t.Fatalf("tanks play: %v\n%s", err, playOut.String())
	}
	if !strings.Contains(playOut.String(), "house:idle") {
		t.Errorf("expected house:idle in output, got:\n%s", playOut.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("replay file: %v", err)
	}
}

func TestTanksPlayBadPackage(t *testing.T) {
	dir := t.TempDir() // no bot.json
	var out bytes.Buffer
	err := runTanks([]string{"play", dir, "house:idle"}, &out, &out)
	if err == nil {
		t.Fatal("expected an error for a directory without bot.json")
	}
	if !strings.Contains(err.Error(), "bot.json") {
		t.Errorf("error should mention bot.json, got: %v", err)
	}
}
