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

// containersWithLabel lists container ids currently matching label=value (docker ps -a, so stopped
// containers count too).
func containersWithLabel(t *testing.T, label string) []string {
	t.Helper()
	out, err := exec.Command("docker", "ps", "-a", "--filter", "label="+label, "-q").CombinedOutput()
	if err != nil {
		t.Fatalf("docker ps: %v: %s", err, out)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// TestDockerLaunchCancelledCtxLeavesNoContainer covers the fix for review round 1's Important finding:
// docker create/cp used to run under Launch's own ctx, so a cancellation landing between the daemon
// creating the container and the local CLI process reporting its id back to Go code (cmd.Cancel kills that
// CLI process, but does not undo the daemon-side container it already asked for) left an orphaned,
// unfindable container. create/cp now run on context.WithoutCancel(ctx) (bounded by dockerSetupTimeout
// instead), and Launch checks ctx.Err() once the container exists and is labeled, removing it itself if
// the caller already gave up.
//
// A cancel() fired the instant Launch is called almost always lands before create's CLI subprocess is even
// spawned (os/exec never starts a process at all against an already-cancelled context), which exercises
// the easy, always-safe case but not the narrow bug this is meant to catch. So each iteration instead
// cancels after a delay spread across the plausible duration of create+cp (0..~90ms, docker create+cp for
// this image typically completing within that range per the timings logged elsewhere in this file),
// giving a good chance that at least some iterations land mid-create or mid-cp — genuinely exercising the
// window the fix addresses — while the rest cover the before/after edges.
//
// Checking is deliberately deferred to the end, after every iteration has run, rather than done right
// after each Launch: when cancel() kills the local `docker create` CLI process before it can report the
// container's id, the daemon can still be in the middle of finishing that create server-side, so the
// container can take a little longer than Launch itself to become visible in `docker ps -a` — checking
// immediately was observed (while developing this test against a deliberately reintroduced version of the
// bug) to sometimes miss a real leak that showed up moments later.
func TestDockerLaunchCancelledCtxLeavesNoContainer(t *testing.T) {
	requireDockerRuntime(t)
	dir, entry := writeStarterKit(t, "python")

	const iterations = 10
	values := make([]string, iterations)
	for i := 0; i < iterations; i++ {
		value := fmt.Sprintf("case9-cancel-%d-%d", os.Getpid(), i)
		values[i] = value
		launcher := DockerLauncher{Image: dockerTestImage, Labels: map[string]string{"arena-bot-test": value}}

		ctx, cancel := context.WithCancel(context.Background())
		delay := time.Duration(i) * 10 * time.Millisecond
		go func() {
			time.Sleep(delay)
			cancel()
		}()

		bot, err := launcher.Launch(ctx, Spec{Dir: dir, Language: "python", Entry: entry})
		if err == nil {
			bot.Close()
		}
	}

	// Give the daemon a moment to finish settling any create it was still processing when its CLI client
	// got killed by a cancellation that landed mid-request.
	time.Sleep(2 * time.Second)

	for i, value := range values {
		if leaked := containersWithLabel(t, "arena-bot-test="+value); len(leaked) != 0 {
			t.Errorf("iteration %d: container(s) leaked for label arena-bot-test=%s: %v", i, value, leaked)
			for _, id := range leaked {
				exec.Command("docker", "rm", "-f", id).Run() //nolint:errcheck
			}
		}
	}
}

// TestRemoveStaleBotContainers checks the periodic safety-net sweep: a container it did not create itself
// (simulating one abandoned by a crashed process, the scenario the previous test's fix cannot fully rule
// out — Launch's create/cp now survive a cancelled ctx precisely so a container like this stays labeled
// and findable) is removed once it's older than the given max age.
//
// The sweep is scoped to this test's own unique "arena-bot-test=<value>" label, not the shared
// botContainerLabel ("arena-bot=1") that every DockerLauncher container carries: sweeping that label with
// maxAge 0 would force-remove every arena-bot=1 container on the machine, including live ones started by
// other tests running concurrently (this was observed to fail intermittently under `go test -count=3`) or
// by a developer's local `make up` stack. Scoping to a label unique to this test's own container, and
// asserting removed == 1 (exactly one container, not merely "at least one"), makes the test correct
// regardless of what else happens to be running alongside it.
func TestRemoveStaleBotContainers(t *testing.T) {
	requireDockerRuntime(t)

	value := fmt.Sprintf("case9-stale-%d", os.Getpid())
	label := "arena-bot-test=" + value
	idRaw, err := exec.Command("docker", "create",
		"--label", botContainerLabel,
		"--label", label,
		dockerTestImage, "true").CombinedOutput()
	if err != nil {
		t.Fatalf("docker create: %v: %s", err, idRaw)
	}
	id := strings.TrimSpace(string(idRaw))
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", id).Run() }) //nolint:errcheck

	// maxAge 0: our own container, already created above, counts as stale regardless of how many
	// milliseconds old it is, so the sweep runs without waiting out the real staleBotContainerAge.
	removed, err := removeStaleBotContainers(context.Background(), label, 0)
	if err != nil {
		t.Fatalf("removeStaleBotContainers: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want exactly 1 (our own labeled container)", removed)
	}

	if leaked := containersWithLabel(t, label); len(leaked) != 0 {
		t.Errorf("container %s still present after removeStaleBotContainers: %v", id, leaked)
	}
}
