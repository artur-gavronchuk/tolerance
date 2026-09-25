package match

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"tolerance/internal/games/tanks"
)

// requireInterpreter skips a test when bin isn't on PATH, unless ARENA_TEST_REQUIRE_DOCKER=1 (CI sets
// that flag and always has both python3 and node), in which case a missing interpreter fails the test.
func requireInterpreter(t *testing.T, bin string) {
	t.Helper()
	if _, err := exec.LookPath(bin); err != nil {
		if os.Getenv("ARENA_TEST_REQUIRE_DOCKER") == "1" {
			t.Fatalf("%s not found in PATH: %v", bin, err)
		}
		t.Skipf("%s not found in PATH: %v", bin, err)
	}
}

func writeFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeStarterKit writes lang's starter kit (tanks.Starter) into a fresh temp dir and returns it along
// with the entry point bot.json declares.
func writeStarterKit(t *testing.T, lang string) (dir, entry string) {
	t.Helper()
	files, err := tanks.Starter(lang)
	if err != nil {
		t.Fatalf("tanks.Starter(%q): %v", lang, err)
	}
	dir = t.TempDir()
	for path, data := range files {
		writeFile(t, dir, path, string(data))
	}
	var manifest struct {
		Entry string `json:"entry"`
	}
	if err := json.Unmarshal(files["bot.json"], &manifest); err != nil {
		t.Fatalf("unmarshal bot.json: %v", err)
	}
	if manifest.Entry == "" {
		t.Fatalf("bot.json has no entry")
	}
	return dir, manifest.Entry
}

// assertStarterWins runs the given starter kit (as a real process) against house:idle over 400 ticks and
// checks it behaves like a healthy, winning bot: it answered almost every tick asked of it and it placed
// first against an opponent that never moves or fires.
func assertStarterWins(t *testing.T, lang, entry, dir string) {
	t.Helper()
	players := []Player{
		{Name: "starter", Spec: Spec{Dir: dir, Language: lang, Entry: entry}},
		{Name: "idle", Spec: Spec{House: "idle"}},
	}
	cfg := Config{Seed: 1, Ticks: 400}

	res, err := Run(context.Background(), WithHouse(ProcessLauncher{}), cfg, players)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	p := res.Players[0]
	if p.Status != StatusOK {
		t.Errorf("status = %q, want %q (stderr: %s)", p.Status, StatusOK, p.Stderr)
	}
	if !p.Ready {
		t.Errorf("ready = false, want true (stderr: %s)", p.Stderr)
	}
	if p.Asked == 0 {
		t.Fatalf("bot was never asked a tick")
	}
	if float64(p.Answered) < 0.95*float64(p.Asked) {
		t.Errorf("answered = %d, asked = %d, want answered >= 0.95*asked", p.Answered, p.Asked)
	}
	if p.Place != 1 {
		t.Errorf("place = %d, want 1", p.Place)
	}
}

func TestPythonStarterPlays(t *testing.T) {
	requireInterpreter(t, "python3")
	dir, entry := writeStarterKit(t, "python")
	assertStarterWins(t, "python", entry, dir)
}

func TestJSStarterPlays(t *testing.T) {
	requireInterpreter(t, "node")
	dir, entry := writeStarterKit(t, "javascript")
	assertStarterWins(t, "javascript", entry, dir)
}

// TestStdoutFlushWithoutExplicitFlush checks that ProcessLauncher runs Python with -u: a bot that prints
// its ready reply without passing flush=True must still be seen promptly, because the interpreter's
// stdout isn't block-buffered in the first place.
func TestStdoutFlushWithoutExplicitFlush(t *testing.T) {
	requireInterpreter(t, "python3")
	dir := t.TempDir()
	writeFile(t, dir, "bot.py", `import sys, json
for raw in sys.stdin:
    raw = raw.strip()
    if not raw:
        continue
    msg = json.loads(raw)
    if msg.get("type") == "start":
        print(json.dumps({"type": "ready"}))
    elif msg.get("type") == "end":
        break
`)

	bot, err := (ProcessLauncher{}).Launch(context.Background(), Spec{Dir: dir, Language: "python", Entry: "bot.py"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer bot.Close()

	if err := bot.Send([]byte(`{"type":"start"}`)); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case line, ok := <-bot.Lines():
		if !ok {
			t.Fatalf("Lines closed before a reply arrived (stderr: %s)", bot.Stderr())
		}
		if typ, ok := messageType(line); !ok || typ != "ready" {
			t.Fatalf("got %q, want a ready message", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("no reply within 3s without an explicit flush (stderr: %s)", bot.Stderr())
	}
}

// TestCloseKillsRunaway (Review Focus 3) checks that a bot that never exits on its own — and leaves a
// child process running behind it — is still fully killed by Close, quickly.
func TestCloseKillsRunaway(t *testing.T) {
	requireInterpreter(t, "python3")
	dir := t.TempDir()
	writeFile(t, dir, "bot.py", `import subprocess
print('{"type":"ready"}', flush=True)
subprocess.Popen(["sleep", "60"])
while True:
    pass
`)

	bot, err := (ProcessLauncher{}).Launch(context.Background(), Spec{Dir: dir, Language: "python", Entry: "bot.py"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	// Close is idempotent, so this cleanup runs harmlessly after the explicit Close below too; it just
	// makes sure the runaway process (and its "sleep 60" child) doesn't outlive the test if an assertion
	// fails first.
	t.Cleanup(func() { bot.Close() })

	select {
	case line, ok := <-bot.Lines():
		if !ok {
			t.Fatalf("bot exited before becoming ready (stderr: %s)", bot.Stderr())
		}
		if typ, ok := messageType(line); !ok || typ != "ready" {
			t.Fatalf("got %q, want a ready message", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("bot never became ready")
	}

	pb, ok := bot.(*processBot)
	if !ok {
		t.Fatalf("bot is %T, want *processBot", bot)
	}
	pid := pb.cmd.Process.Pid

	start := time.Now()
	if err := bot.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 2*time.Second {
		t.Errorf("Close took %s, want < 2s", elapsed)
	}

	if err := syscall.Kill(-pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Errorf("process group %d still alive after Close: %v", pid, err)
	}
}

// TestStderrCapped checks that Stderr() keeps only the last 16 KiB written, not the first.
func TestStderrCapped(t *testing.T) {
	requireInterpreter(t, "python3")
	dir := t.TempDir()
	writeFile(t, dir, "bot.py", `import sys
sys.stderr.write("HEAD" + "a" * (100 * 1024) + "TAIL")
sys.stderr.flush()
`)

	bot, err := (ProcessLauncher{}).Launch(context.Background(), Spec{Dir: dir, Language: "python", Entry: "bot.py"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer bot.Close()

	select {
	case _, ok := <-bot.Lines():
		if ok {
			t.Fatalf("unexpected stdout line from a bot that only writes to stderr")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("bot did not exit in time")
	}

	stderr := bot.Stderr()
	const cap = 16 * 1024
	if len(stderr) > cap {
		t.Errorf("stderr len = %d, want <= %d", len(stderr), cap)
	}
	if !strings.HasSuffix(stderr, "TAIL") {
		t.Errorf("stderr does not end with the last bytes written: %q", tail(stderr, 40))
	}
	if strings.Contains(stderr, "HEAD") {
		t.Errorf("stderr still contains bytes from the start of a 100 KiB write")
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
