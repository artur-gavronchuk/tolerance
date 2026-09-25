package match

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// dockerTestImage matches the tag the Makefile's bot-image target and the CI step build.
const dockerTestImage = "arena-bot-runtime:1"

var (
	buildRuntimeOnce sync.Once
	buildRuntimeErr  error
)

// requireDockerRuntime skips (or, under ARENA_TEST_REQUIRE_DOCKER=1, fails) when docker itself is
// unreachable, exactly like proofs/sandbox's requireDocker. Unlike that helper, it builds the runtime
// image only once per test binary run (sync.Once): the image's COPY --from=node:22-slim pulls a second
// base image on a cold cache, which is too slow to redo in every one of this file's tests.
func requireDockerRuntime(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		if os.Getenv("ARENA_TEST_REQUIRE_DOCKER") == "1" {
			t.Fatalf("docker unavailable: %v", err)
		}
		t.Skipf("docker unavailable: %v", err)
	}
	buildRuntimeOnce.Do(func() {
		out, err := exec.Command("docker", "build", "-q", "-t", dockerTestImage, "runtime").CombinedOutput()
		if err != nil {
			buildRuntimeErr = fmt.Errorf("build runtime image: %w\n%s", err, out)
		}
	})
	if buildRuntimeErr != nil {
		t.Fatalf("%v", buildRuntimeErr)
	}
}

// assertDockerStarterWins is assertStarterWins (process_test.go) played through DockerLauncher instead of
// ProcessLauncher, over 300 ticks per the plan's brief (a full 400-tick, host-process match would just add
// container overhead without testing anything new).
func assertDockerStarterWins(t *testing.T, lang, entry, dir string) {
	t.Helper()
	players := []Player{
		{Name: "starter", Spec: Spec{Dir: dir, Language: lang, Entry: entry}},
		{Name: "idle", Spec: Spec{House: "idle"}},
	}
	cfg := Config{Seed: 1, Ticks: 300}

	start := time.Now()
	res, err := Run(context.Background(), WithHouse(DockerLauncher{Image: dockerTestImage}), cfg, players)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Logf("300-tick Docker match wall time: %s", elapsed)

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

func TestDockerPythonStarterPlays(t *testing.T) {
	requireDockerRuntime(t)
	dir, entry := writeStarterKit(t, "python")
	assertDockerStarterWins(t, "python", entry, dir)
}

func TestDockerJSStarterPlays(t *testing.T) {
	requireDockerRuntime(t)
	dir, entry := writeStarterKit(t, "javascript")
	assertDockerStarterWins(t, "javascript", entry, dir)
}

// TestDockerNoNetwork checks that --network none actually holds: a bot that tries to reach the public
// internet the moment it starts gets a connection error, reports it to stderr, and — since the tanks
// protocol has no concept of "networking failed", only "no reply in time" — keeps playing normally
// afterward.
func TestDockerNoNetwork(t *testing.T) {
	requireDockerRuntime(t)
	dir := t.TempDir()
	writeFile(t, dir, "bot.py", `import json
import socket
import sys

try:
    socket.create_connection(("1.1.1.1", 53), 1)
    print("NETCHECK: connected", file=sys.stderr, flush=True)
except OSError as e:
    print("NETCHECK: " + repr(e), file=sys.stderr, flush=True)

for raw in sys.stdin:
    raw = raw.strip()
    if not raw:
        continue
    msg = json.loads(raw)
    kind = msg.get("type")
    if kind == "start":
        print(json.dumps({"type": "ready"}), flush=True)
    elif kind == "tick":
        print(json.dumps({"move": 0, "turn": 0, "turret": 0, "fire": False, "tick": msg["tick"]}), flush=True)
    elif kind == "end":
        break
`)

	players := []Player{
		{Name: "netcheck", Spec: Spec{Dir: dir, Language: "python", Entry: "bot.py"}},
		{Name: "idle", Spec: Spec{House: "idle"}},
	}
	cfg := Config{Seed: 1, Ticks: 30}

	res, err := Run(context.Background(), WithHouse(DockerLauncher{Image: dockerTestImage}), cfg, players)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	p := res.Players[0]
	if !strings.Contains(p.Stderr, "NETCHECK:") {
		t.Fatalf("stderr has no NETCHECK line: %q", p.Stderr)
	}
	if strings.Contains(p.Stderr, "NETCHECK: connected") {
		t.Fatalf("bot reached the public internet from inside the sandbox: %q", p.Stderr)
	}
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
}

// TestDockerCloseKillsRunaway is Review Focus 3 for the Docker launcher: a bot that never exits on its own
// must still be fully gone — its container removed — soon after Close, even though (unlike a local
// process) an exited container isn't reaped by the OS and must be removed explicitly. It also cancels the
// match's own context before calling Close, to check that container cleanup does not depend on that
// context still being live (e.g. the worker's own ctx was already cancelled when the match's bound expired
// or the request that started it was aborted).
func TestDockerCloseKillsRunaway(t *testing.T) {
	requireDockerRuntime(t)
	dir := t.TempDir()
	writeFile(t, dir, "bot.py", `import sys
print('{"type":"ready"}')
sys.stdout.flush()
while True:
    pass
`)

	ctx, cancel := context.WithCancel(context.Background())
	bot, err := (DockerLauncher{Image: dockerTestImage}).Launch(ctx, Spec{Dir: dir, Language: "python", Entry: "bot.py"})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	// Close is idempotent, so this harmlessly no-ops after the explicit Close below on any early return.
	t.Cleanup(func() { bot.Close() })

	select {
	case line, ok := <-bot.Lines():
		if !ok {
			t.Fatalf("bot exited before becoming ready (stderr: %s)", bot.Stderr())
		}
		if typ, ok := messageType(line); !ok || typ != "ready" {
			t.Fatalf("got %q, want a ready message", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("bot never became ready")
	}

	db, ok := bot.(*dockerBot)
	if !ok {
		t.Fatalf("bot is %T, want *dockerBot", bot)
	}
	id := db.id

	// Mid-match cancellation: Launch's ctx is done, but Close must still clean up on its own context.
	cancel()

	start := time.Now()
	if err := bot.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Errorf("Close took %s, want < 5s", elapsed)
	}

	out, err := exec.Command("docker", "ps", "-a", "--filter", "id="+id, "-q").CombinedOutput()
	if err != nil {
		t.Fatalf("docker ps: %v: %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("container %s still present after Close: %s", id, out)
	}
}
