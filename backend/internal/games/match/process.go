package match

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	linesBuf    = 64
	stderrCap   = 16 * 1024
	sendTimeout = 1 * time.Second
	closeGrace  = 200 * time.Millisecond
)

// ProcessLauncher runs a bot as a local process in Spec.Dir with the argv from Command, in its own process
// group so Close kills everything it started. Used by the connector, by tests and by ARENA_SANDBOX=fake.
// Never use it for untrusted code on the server.
type ProcessLauncher struct{}

// Launch starts the bot. An error here means the platform failed to start it at all — most commonly the
// interpreter (python3, node) isn't on PATH, which exec.Command surfaces as an error from cmd.Start(). A
// bot whose own code fails immediately after that is a Bot whose Lines() channel closes right away, not a
// Launch error.
func (ProcessLauncher) Launch(ctx context.Context, s Spec) (Bot, error) {
	argv, err := Command(s.Language, s.Entry)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = s.Dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + s.Dir,
		"PYTHONDONTWRITEBYTECODE=1",
		"LANG=C.UTF-8",
	}
	// Own process group: Close kills the bot and anything it spawned (SIGKILL -pid), not just this one
	// process.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("match: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("match: stdout pipe: %w", err)
	}
	// cmd.Stderr, unlike Stdout, is left as a plain io.Writer: os/exec starts its own internal
	// copy-and-close goroutine for it and folds waiting for that goroutine into cmd.Wait(), so the tail
	// buffer needs no separate management here.
	tail := newTailWriter(stderrCap)
	cmd.Stderr = tail

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("match: start %v: %w", argv, err)
	}

	b := &processBot{
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

// processBot is a Bot backed by a real OS process, speaking the tanks protocol over its stdin/stdout.
type processBot struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan []byte
	tail  *tailWriter

	waited  chan struct{} // closed once stdout has been fully drained and cmd.Wait() has returned
	closing chan struct{} // closed by the first Close, so a full lines buffer stops blocking readStdout

	closeOnce sync.Once
}

// readStdout drains stdout line by line until it ends, delivering each line on Lines (dropping it instead
// of blocking once Close has started shutting things down, so this goroutine can't get stuck forever on a
// full, unread buffer). It then reaps the process — cmd.Wait() must not run concurrently with reading
// StdoutPipe, so this single goroutine does both, in order — and signals completion via waited.
func (b *processBot) readStdout(stdout io.Reader) {
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
		// A final line with no trailing newline comes back alongside err == nil (see readLine), so
		// reaching here with err != nil means line is nil: nothing left to deliver, just stop.
		break
	}
	close(b.lines)
	_ = b.cmd.Wait()
	close(b.waited)
}

func (b *processBot) Send(line []byte) error {
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

func (b *processBot) Lines() <-chan []byte { return b.lines }

func (b *processBot) Stderr() string { return b.tail.String() }

// Close closes stdin (a clean shutdown signal an obedient bot can react to on its own), gives it
// closeGrace to exit, and then kills the whole process group so a bot that ignores end / stdin closing —
// or leaves children running behind it — cannot outlive the match. It waits for the process to be reaped
// and for readStdout to finish before returning, and is safe to call more than once (concurrent callers
// block until the first Close has finished, courtesy of sync.Once).
func (b *processBot) Close() error {
	b.closeOnce.Do(func() {
		close(b.closing)
		_ = b.stdin.Close()

		select {
		case <-b.waited:
			return
		case <-time.After(closeGrace):
		}

		if p := b.cmd.Process; p != nil {
			_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
		}
		<-b.waited
	})
	return nil
}
