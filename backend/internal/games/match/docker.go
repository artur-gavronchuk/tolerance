package match

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// dockerCloseGrace mirrors closeGrace (process.go): time given to a bot that closed its stdin cleanly to
// exit on its own before Close force-removes its container.
const dockerCloseGrace = 300 * time.Millisecond

// dockerRemoveTimeout bounds every `docker rm -f`, run on a context that is never the caller's — a match's
// own ctx may already be cancelled (Close) or the daemon may just be slow — so container cleanup is never
// skipped for that reason, but a genuinely wedged daemon still can't hang a caller forever.
const dockerRemoveTimeout = 30 * time.Second

// dockerSetupTimeout bounds `docker create` and `docker cp` in Launch. They run on a context that
// deliberately survives the caller's ctx being cancelled (see Launch), so this is what keeps a wedged
// daemon from hanging Launch forever instead.
const dockerSetupTimeout = 60 * time.Second

// botContainerLabel is attached to every container DockerLauncher creates, so a container that Launch or
// Close failed to clean up (the process running them was killed, most commonly) can still be found and
// removed later by RemoveStaleBotContainers.
const botContainerLabel = "arena-bot=1"

// staleBotContainerAge is how old a labeled container must be before RemoveStaleBotContainers treats it as
// abandoned. A match's own Close always removes its container immediately on a normal run; this is only a
// safety net for the cases that can't reach, so it can afford to be generous.
const staleBotContainerAge = 30 * time.Minute

// DockerLauncher runs each bot in its own throwaway container: no network, 256 MiB, half a CPU, 64 pids,
// all capabilities dropped, no-new-privileges, uid 65534, 16 MiB tmpfs /tmp. Code is copied in with
// docker cp (the API may itself run in a container, so a bind mount of Spec.Dir would not be visible to
// the daemon — the same reasoning as internal/proofs/sandbox.Docker), stdin/stdout/stderr are attached
// with docker start -ai. Never use it for anything but untrusted bot code; ProcessLauncher is for the
// connector and for trusted house-adjacent uses.
//
// Every container is created with the label "arena-bot=1" (see botContainerLabel and
// RemoveStaleBotContainers); Labels adds further labels on top of that — tests use it to tag their own
// containers with a unique value so they can assert none of theirs are left behind without disturbing
// containers from other concurrent tests or matches.
type DockerLauncher struct {
	Image  string
	Labels map[string]string
}

// image returns the configured image, defaulting to the one the Makefile and CI build.
func (d DockerLauncher) image() string {
	if d.Image != "" {
		return d.Image
	}
	return "arena-bot-runtime:1"
}

func (d DockerLauncher) labelArgs() []string {
	args := make([]string, 0, 2+2*len(d.Labels))
	args = append(args, "--label", botContainerLabel)
	for k, v := range d.Labels {
		args = append(args, "--label", k+"="+v)
	}
	return args
}

// removeContainer force-removes id, bounded by dockerRemoveTimeout on top of ctx (which may itself already
// be Background() with no deadline, in callers that want an independent cleanup).
func removeContainer(ctx context.Context, id string) error {
	rmCtx, cancel := context.WithTimeout(ctx, dockerRemoveTimeout)
	defer cancel()
	return exec.CommandContext(rmCtx, "docker", "rm", "-f", id).Run()
}

// Launch creates a container for s, copies s.Dir into it and starts it attached. An error here means the
// platform failed (docker create/cp/start itself failing, e.g. the image is missing) — same contract as
// ProcessLauncher.Launch and internal/proofs/sandbox.Docker.Run.
func (d DockerLauncher) Launch(ctx context.Context, s Spec) (Bot, error) {
	argv, err := Command(s.Language, s.Entry)
	if err != nil {
		return nil, err
	}

	createArgs := append([]string{"create", "-i",
		"--network", "none",
		"--memory", "256m", "--memory-swap", "256m",
		"--cpus", "0.5",
		"--pids-limit", "64",
		"--cap-drop=ALL", "--security-opt=no-new-privileges",
		"--user", "65534:65534",
		"--tmpfs", "/tmp:rw,size=16m",
	}, d.labelArgs()...)
	createArgs = append(createArgs, "-w", "/bot", d.image())
	createArgs = append(createArgs, argv...)

	// create and cp run on a context that survives ctx being cancelled: if ctx were used directly and got
	// cancelled while the daemon was still creating the container but before the CLI had printed its id
	// back to us, we would have no id to clean up with — an orphaned, unfindable container. Bounded
	// separately (dockerSetupTimeout) so a genuinely wedged daemon still can't hang Launch forever, and the
	// label above means RemoveStaleBotContainers can find and remove it even in that case.
	setupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dockerSetupTimeout)
	defer cancel()

	idRaw, err := exec.CommandContext(setupCtx, "docker", createArgs...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("match: docker create: %w: %s", err, strings.TrimSpace(string(idRaw)))
	}
	id := strings.TrimSpace(string(idRaw))

	// docker cp works against a created-but-not-started container; the bot's own files land in /bot
	// owned by whatever uid ran this process (not 65534 — docker cp does not remap ownership to the
	// container's user), but they keep the mode they had on disk (0644/0755 from unpacking), which is
	// world-readable/executable, so uid 65534 can still read and run them.
	if out, err := exec.CommandContext(setupCtx, "docker", "cp", s.Dir+"/.", id+":/bot").CombinedOutput(); err != nil {
		_ = removeContainer(context.Background(), id)
		return nil, fmt.Errorf("match: docker cp: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// The container exists and is labeled now, so even a cancelled ctx from here on leaves it findable by
	// RemoveStaleBotContainers as a fallback — but check here anyway so a Launch whose ctx died during
	// setup doesn't go on to start the bot and hand back a Bot nobody asked for.
	if err := ctx.Err(); err != nil {
		_ = removeContainer(context.Background(), id)
		return nil, err
	}

	// Deliberately not tied to ctx from here on: if the match's context is cancelled mid-run, killing this
	// CLI process would leave the container itself running in the daemon (the CLI is just an attached
	// client). Close is what tears the container down, on its own fresh context, regardless of ctx's fate.
	cmd := exec.Command("docker", "start", "-ai", id)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = removeContainer(context.Background(), id)
		return nil, fmt.Errorf("match: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = removeContainer(context.Background(), id)
		return nil, fmt.Errorf("match: stdout pipe: %w", err)
	}
	// Same tailWriter used for process stderr (lines.go): os/exec runs its own copy-and-close goroutine
	// for a plain io.Writer Stderr, folded into cmd.Wait().
	tail := newTailWriter(stderrCap)
	cmd.Stderr = tail

	if err := cmd.Start(); err != nil {
		_ = removeContainer(context.Background(), id)
		return nil, fmt.Errorf("match: docker start: %w", err)
	}

	b := &dockerBot{
		id:      id,
		cmd:     cmd,
		stdin:   stdin,
		lines:   make(chan []byte, linesBuf),
		tail:    tail,
		waited:  make(chan struct{}),
		closing: make(chan struct{}),
	}
	go b.readStdout(stdout)
	return b, nil
}

// dockerPSCreatedAtLayout matches `docker ps --format '{{.CreatedAt}}'`, e.g.
// "2026-09-25 05:06:08 +0300 MSK".
const dockerPSCreatedAtLayout = "2006-01-02 15:04:05 -0700 MST"

// RemoveStaleBotContainers force-removes every container labeled arena-bot=1 (see botContainerLabel) that
// is older than staleBotContainerAge, and returns how many it removed. DockerLauncher.Close always removes
// its own container right after a match; this is a periodic safety net (the games worker is meant to call
// it hourly) for the containers that can't reach — most commonly one whose owning process was killed
// before Close ran, or one orphaned by Launch's own ctx being cancelled during docker create.
func RemoveStaleBotContainers(ctx context.Context) (int, error) {
	return removeStaleBotContainers(ctx, staleBotContainerAge)
}

// removeStaleBotContainers is RemoveStaleBotContainers with an injectable age, so tests can exercise it
// against a container they just created without waiting out the real staleBotContainerAge.
func removeStaleBotContainers(ctx context.Context, maxAge time.Duration) (int, error) {
	out, err := exec.CommandContext(ctx, "docker", "ps", "-a",
		"--filter", "label="+botContainerLabel,
		"--format", "{{.ID}}\t{{.CreatedAt}}").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("match: docker ps: %w: %s", err, strings.TrimSpace(string(out)))
	}

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 {
			continue
		}
		id, createdAt := fields[0], fields[1]
		created, err := time.Parse(dockerPSCreatedAtLayout, createdAt)
		if err != nil || created.After(cutoff) {
			continue
		}
		if err := removeContainer(ctx, id); err == nil {
			removed++
		}
	}
	return removed, nil
}

// dockerBot is a Bot backed by `docker start -ai <id>`, speaking the tanks protocol over its attached
// stdin/stdout exactly like processBot does over a local process's pipes.
type dockerBot struct {
	id    string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan []byte
	tail  *tailWriter

	waited  chan struct{} // closed once stdout has been fully drained and cmd.Wait() has returned
	closing chan struct{} // closed by the first Close, so a full lines buffer stops blocking readStdout

	closeOnce sync.Once
}

// readStdout mirrors processBot.readStdout: drain stdout line by line, then reap the attached CLI process
// (cmd.Wait() must not run concurrently with reading StdoutPipe, hence one goroutine doing both).
//
// Unlike ProcessLauncher, whose cmd.Start() failing is itself the platform-failure signal, a Docker
// container that fails to run at all (missing binary inside the image, broken entrypoint) is only
// discovered here, once `docker start` exits — Launch has already returned a Bot by then, so it can't be
// turned into a Launch error. Exit code 125 is docker's own signal for exactly that case (see the comment
// above sandbox.Docker.Run for the same reasoning); it's recorded in Stderr so the failure isn't silently
// misattributed to the bot's own code when Run marks it crashed. Verified empirically (docker 29.8.1
// client / 28.4.0 server): current `docker start -a` actually exits 1, not 125, for a container that fails
// to run — the real 127-style exit code only shows up via `docker inspect`. This branch is kept for older
// or differently-behaving daemons; on this Docker it mostly won't fire, so don't rely on it alone.
func (b *dockerBot) readStdout(stdout io.Reader) {
	r := bufio.NewReaderSize(stdout, 4096)
	for {
		line, err := readLine(r, maxLineBytes)
		if err == nil {
			select {
			case b.lines <- line:
			case <-b.closing:
			}
			continue
		}
		break
	}
	close(b.lines)
	err := b.cmd.Wait()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 125 {
		fmt.Fprintf(b.tail, "\nmatch: docker start exited 125: the container failed to run (a platform failure, not the bot's)\n")
	}
	close(b.waited)
}

func (b *dockerBot) Send(line []byte) error {
	out := make([]byte, len(line)+1)
	copy(out, line)
	out[len(line)] = '\n'

	errc := make(chan error, 1)
	go func() {
		_, err := b.stdin.Write(out)
		errc <- err
	}()

	select {
	case err := <-errc:
		return err
	case <-time.After(sendTimeout):
		return fmt.Errorf("match: stdin write timed out after %s", sendTimeout)
	}
}

func (b *dockerBot) Lines() <-chan []byte { return b.lines }

func (b *dockerBot) Stderr() string { return b.tail.String() }

// Close closes stdin (a clean shutdown signal), gives the bot dockerCloseGrace to exit on its own, and
// then always force-removes the container: unlike a local process, an exited container is not reaped by
// the OS on its own — `docker create` (without --rm) leaves a stopped container behind, so Close must
// remove it explicitly even when the bot exited cleanly, or every match would leak one container. Removal
// runs on a fresh, uncancelled context so a match whose own ctx was already cancelled still cleans up.
// Close waits for the attached CLI process and the stdout reader to finish before returning, and is safe
// to call more than once.
func (b *dockerBot) Close() error {
	b.closeOnce.Do(func() {
		close(b.closing)
		_ = b.stdin.Close()

		select {
		case <-b.waited:
		case <-time.After(dockerCloseGrace):
		}

		_ = removeContainer(context.Background(), b.id)

		<-b.waited
	})
	return nil
}
