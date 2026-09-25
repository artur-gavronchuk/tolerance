package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"tolerance/internal/games/botpkg"
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

// setArenaHome points ARENA_HOME at a fresh temp dir with a config.yaml (url = the given server URL,
// agent.command set to satisfy loadConfig) and a key file, restoring the previous value on cleanup.
func setArenaHome(t *testing.T, url string) {
	t.Helper()
	dir := t.TempDir()
	cfg := "url: " + url + "\nagent:\n  command: \"true\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key"), []byte("ak_test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARENA_HOME", dir)
}

func TestTanksSubmit(t *testing.T) {
	botDir := filepath.Join(t.TempDir(), "mybot")
	var newOut bytes.Buffer
	if err := runTanks([]string{"new", botDir, "--lang", "python"}, &newOut, &newOut); err != nil {
		t.Fatalf("tanks new: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/connector/tanks/versions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ak_test" {
			t.Fatalf("Authorization = %q", got)
		}
		var in struct {
			ArchiveBase64 string `json:"archive_base64"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		archive, err := base64.StdEncoding.DecodeString(in.ArchiveBase64)
		if err != nil {
			t.Fatalf("archive_base64: %v", err)
		}
		if _, err := botpkg.Validate(archive); err != nil {
			t.Fatalf("uploaded archive failed validation: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "bv1", "number": 1, "source": "upload", "status": "pending",
			"language": "python", "checks": []any{}, "check_log": "",
			"check_match_id": nil, "proof_id": nil, "created_at": "2026-09-25T10:00:00Z",
		})
	}))
	defer srv.Close()
	setArenaHome(t, srv.URL)

	var out bytes.Buffer
	if err := runTanks([]string{"submit", botDir}, &out, &out); err != nil {
		t.Fatalf("tanks submit: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "version 1") {
		t.Errorf("expected output to mention version 1, got %q", got)
	}
	if !strings.Contains(got, srv.URL+"/app/tanks") {
		t.Errorf("expected output to link to %s/app/tanks, got %q", srv.URL, got)
	}
}

func TestTanksSubmitInvalid(t *testing.T) {
	dir := t.TempDir() // no bot.json
	var out bytes.Buffer
	err := runTanks([]string{"submit", dir}, &out, &out)
	if err == nil {
		t.Fatal("expected an error for a directory without bot.json")
	}
	if !strings.Contains(err.Error(), "bot.json") {
		t.Errorf("error should mention bot.json, got: %v", err)
	}
}

func TestTanksSubmitServerRejectsPackage(t *testing.T) {
	botDir := filepath.Join(t.TempDir(), "mybot")
	var newOut bytes.Buffer
	if err := runTanks([]string{"new", botDir, "--lang", "python"}, &newOut, &newOut); err != nil {
		t.Fatalf("tanks new: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"code": "invalid_package", "message": "bot.json: unsupported language",
		})
	}))
	defer srv.Close()
	setArenaHome(t, srv.URL)

	var out bytes.Buffer
	err := runTanks([]string{"submit", botDir}, &out, &out)
	if err == nil {
		t.Fatal("expected an error when the server rejects the package")
	}
	if !strings.Contains(err.Error(), "bot.json: unsupported language") {
		t.Errorf("expected the server's message, got: %v", err)
	}
}

func TestTanksSubmitBadKey(t *testing.T) {
	botDir := filepath.Join(t.TempDir(), "mybot")
	var newOut bytes.Buffer
	if err := runTanks([]string{"new", botDir, "--lang", "python"}, &newOut, &newOut); err != nil {
		t.Fatalf("tanks new: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "unauthorized", "message": "bad key"})
	}))
	defer srv.Close()
	setArenaHome(t, srv.URL)

	var out bytes.Buffer
	err := runTanks([]string{"submit", botDir}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "arena login") {
		t.Fatalf("expected an API-key-rejected error, got: %v", err)
	}
}
