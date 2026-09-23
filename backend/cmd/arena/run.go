package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"tolerance/internal/platform/sanitize"
	"tolerance/internal/proofs"
)

const logTailBytes = 32 << 10

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=arena", "GIT_AUTHOR_EMAIL=arena@localhost",
		"GIT_COMMITTER_NAME=arena", "GIT_COMMITTER_EMAIL=arena@localhost", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("git %v: %w: %s", args, err, bytes.TrimSpace(out))
	}
	return out, nil
}

// runTask unpacks the repo, runs the owner's agent command against it and
// returns the diff of what the agent changed. Nothing the agent prints
// leaves this machine except the redacted tail in result.LogTail.
func runTask(ctx context.Context, task nextTask, repo []byte, command string) (result, error) {
	sum := sha256.Sum256(repo)
	if hex.EncodeToString(sum[:]) != task.Task.RepoSHA256 {
		return result{}, errors.New("repository tarball checksum mismatch")
	}
	dir, err := os.MkdirTemp("", "arena-"+task.Task.Slug+"-")
	if err != nil {
		return result{}, err
	}
	defer os.RemoveAll(dir)
	if err := proofs.Untar(repo, dir); err != nil {
		return result{}, err
	}
	if _, err := git(ctx, dir, "init", "-q"); err != nil {
		return result{}, err
	}
	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return result{}, err
	}
	if _, err := git(ctx, dir, "commit", "-q", "-m", "task"); err != nil {
		return result{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "TASK.md"), []byte(task.Task.TaskMD), 0o644); err != nil {
		return result{}, err
	}

	logFile, err := os.Create(filepath.Join(os.TempDir(), "arena-"+task.ProofID+".log"))
	if err != nil {
		return result{}, err
	}
	defer logFile.Close()

	timeout := time.Duration(task.Task.AgentTimeoutS) * time.Second
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Env = append(os.Environ(), "ARENA_TASK="+task.Task.Slug, "ARENA_PROOF="+task.ProofID)
	start := time.Now()
	runErr := cmd.Run()
	res := result{DurationMS: int(time.Since(start).Milliseconds())}
	var exitErr *exec.ExitError
	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		res.TimedOut, res.ExitCode = true, -1
	case runErr == nil:
		res.ExitCode = 0
	case errors.As(runErr, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		return result{}, fmt.Errorf("start agent command: %w", runErr)
	}

	raw, _ := os.ReadFile(logFile.Name())
	res.LogTail = sanitize.CleanLog(string(raw), logTailBytes)

	_ = os.Remove(filepath.Join(dir, "TASK.md"))
	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return result{}, err
	}
	diff, err := git(ctx, dir, "diff", "--cached", "--no-color")
	if err != nil {
		return result{}, err
	}
	res.Diff = string(diff)
	return res, nil
}
