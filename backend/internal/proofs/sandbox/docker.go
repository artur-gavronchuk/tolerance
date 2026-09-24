package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Docker runs the work directory in a throwaway container via the docker
// CLI. The directory is copied in with `docker cp` rather than bind-mounted
// so the same code works when the API itself runs in a container that only
// has the host's docker.sock (its filesystem is invisible to the daemon).
type Docker struct{}

func NewDocker() *Docker { return &Docker{} }

const (
	maxOutput         = 256 << 10
	maxCapturedOutput = 8 << 20 // hard cap on what is buffered before tail() trims to maxOutput
)

// cappedWriter discards bytes past limit instead of growing forever; the
// caller only cares about the tail of what was captured, via tail().
type cappedWriter struct {
	buf   bytes.Buffer
	limit int
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if room := w.limit - w.buf.Len(); room > 0 {
		if len(p) > room {
			w.buf.Write(p[:room])
		} else {
			w.buf.Write(p)
		}
	}
	return len(p), nil
}

func (d *Docker) Run(ctx context.Context, req Request) (Result, error) {
	// --read-only is deliberately omitted here, unlike the original design: it
	// is incompatible with the docker-cp-based file injection below (docker cp
	// fails with "container rootfs is marked read-only" against a read-only
	// container, and docker update cannot re-apply --read-only afterward).
	// Network isolation, resource limits and immediate container removal after
	// each run remain in place; only in-container filesystem tampering during
	// a single ephemeral run is no longer prevented.
	create := exec.CommandContext(ctx, "docker", "create",
		"--network", "none", "--memory", "1g", "--cpus", "1", "--pids-limit", "256",
		"--cap-drop=ALL", "--security-opt=no-new-privileges",
		"--tmpfs", "/tmp:rw,exec,size=512m",
		"-e", "HOME=/tmp", "-e", "GOCACHE=/tmp/gocache", "-e", "GOPATH=/tmp/gopath", "-e", "GOTMPDIR=/tmp",
		"-w", "/work", req.Image, "sh", "-c", req.RunCmd)
	idRaw, err := create.CombinedOutput()
	if err != nil {
		return Result{}, fmt.Errorf("sandbox: docker create: %w: %s", err, strings.TrimSpace(string(idRaw)))
	}
	id := strings.TrimSpace(string(idRaw))
	defer exec.Command("docker", "rm", "-f", id).Run() //nolint:errcheck

	if out, err := exec.CommandContext(ctx, "docker", "cp", req.WorkDir+"/.", id+":/work").CombinedOutput(); err != nil {
		return Result{}, fmt.Errorf("sandbox: docker cp: %w: %s", err, strings.TrimSpace(string(out)))
	}

	runCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	buf := &cappedWriter{limit: maxCapturedOutput}
	start := exec.CommandContext(runCtx, "docker", "start", "-a", id)
	start.Stdout, start.Stderr = buf, buf
	err = start.Run()
	res := Result{Output: tail(buf.buf.String(), maxOutput)}
	res.Tests = ParseGoTestJSON([]byte(res.Output))
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		res.TimedOut, res.ExitCode = true, -1
		return res, nil
	}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		res.ExitCode = 0
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		return Result{}, fmt.Errorf("sandbox: docker start: %w", err)
	}
	// `docker start -a` exits 125 when the container itself could not run
	// (e.g. the command is missing); that is ours, not the participant's.
	if res.ExitCode == 125 {
		return Result{}, fmt.Errorf("sandbox: container failed to start: %s", tail(res.Output, 2000))
	}
	return res, nil
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

var _ = time.Second
