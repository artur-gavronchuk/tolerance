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

// dockerRemoveTimeout bounds `docker rm -f` in Close, which runs on a fresh context (not the match's,
// which may already be cancelled) so container cleanup isn't skipped just because the match ended.
const dockerRemoveTimeout = 30 * time.Second

// DockerLauncher runs each bot in its own throwaway container: no network, 256 MiB, half a CPU, 64 pids,
// all capabilities dropped, no-new-privileges, uid 65534, 16 MiB tmpfs /tmp. Code is copied in with
// docker cp (the API may itself run in a container, so a bind mount of Spec.Dir would not be visible to
// the daemon — the same reasoning as internal/proofs/sandbox.Docker), stdin/stdout/stderr are attached
// with docker start -ai. Never use it for anything but untrusted bot code; ProcessLauncher is for the
// connector and for trusted house-adjacent uses.
type DockerLauncher struct{ Image string }

// image returns the configured image, defaulting to the one the Makefile and CI build.
func (d DockerLauncher) image() string {
	if d.Image != "" {
		return d.Image
	}
	return "arena-bot-runtime:1"
}

// Launch creates a container for s, copies s.Dir into it and starts it attached. An error here means the
// platform failed (docker create/cp/start itself failing, e.g. the image is missing) — same contract as
// ProcessLauncher.Launch and internal/proofs/sandbox.Docker.Run.
func (d DockerLauncher) Launch(ctx context.Context, s Spec) (Bot, error) {
	argv, err := Command(s.Language, s.Entry)
	if err != nil {
		return nil, err
	}

	createArgs := append([]string{
		"create", "-i",
		"--network", "none",
		"--memory", "256m", "--memory-swap", "256m",
		"--cpus", "0.5",
		"--pids-limit", "64",
		"--cap-drop=ALL", "--security-opt=no-new-privileges",
		"--user", "65534:65534",
		"--tmpfs", "/tmp:rw,size=16m",
		"-w", "/bot",
		d.image(),
	}, argv...)
	idRaw, err := exec.CommandContext(ctx, "docker", createArgs...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("match: docker create: %w: %s", err, strings.TrimSpace(string(idRaw)))
	}
	id := strings.TrimSpace(string(idRaw))

	// docker cp works against a created-but-not-started container; the bot's own files land in /bot
	// owned by whatever uid ran this process (not 65534 — docker cp does not remap ownership to the
	// container's user), but they keep the mode they had on disk (0644/0755 from unpacking), which is
	// world-readable/executable, so uid 65534 can still read and run them.
	if out, err := exec.CommandContext(ctx, "docker", "cp", s.Dir+"/.", id+":/bot").CombinedOutput(); err != nil {
		exec.Command("docker", "rm", "-f", id).Run() //nolint:errcheck
		return nil, fmt.Errorf("match: docker cp: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// Deliberately not tied to ctx: if the match's context is cancelled mid-run, killing this CLI process
	// would leave the container itself running in the daemon (the CLI is just an attached client). Close
	// is what tears the container down, on its own fresh context, regardless of ctx's fate.
	cmd := exec.Command("docker", "start", "-ai", id)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		exec.Command("docker", "rm", "-f", id).Run() //nolint:errcheck
		return nil, fmt.Errorf("match: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		exec.Command("docker", "rm", "-f", id).Run() //nolint:errcheck
		return nil, fmt.Errorf("match: stdout pipe: %w", err)
	}
	// Same tailWriter used for process stderr (lines.go): os/exec runs its own copy-and-close goroutine
	// for a plain io.Writer Stderr, folded into cmd.Wait().
	tail := newTailWriter(stderrCap)
	cmd.Stderr = tail

	if err := cmd.Start(); err != nil {
		exec.Command("docker", "rm", "-f", id).Run() //nolint:errcheck
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
// misattributed to the bot's own code when Run marks it crashed.
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

		rmCtx, cancel := context.WithTimeout(context.Background(), dockerRemoveTimeout)
		defer cancel()
		_ = exec.CommandContext(rmCtx, "docker", "rm", "-f", b.id).Run()

		<-b.waited
	})
	return nil
}
