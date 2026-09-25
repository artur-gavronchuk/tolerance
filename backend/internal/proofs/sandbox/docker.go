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
		// --storage-opt size= would cap the writable layer directly but only
		// works on a handful of storage drivers (devicemapper, btrfs, zfs,
		// overlay2-on-xfs-with-pquota) and errors out on anything else, so it
		// cannot be turned on unconditionally here. --ulimit fsize caps how
		// large a single file the sandboxed process may create, everywhere:
		// a malicious diff or test cannot fill the host disk by writing one
		// huge file into /work (the container's writable layer, not the
		// size-capped /tmp above). It does not cap many small files summing
		// past this, but combined with --memory and the timeout in Run, that
		// residual risk is bounded by how much a single sandbox run can do
		// before being killed.
		"--ulimit", "fsize=209715200",
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
