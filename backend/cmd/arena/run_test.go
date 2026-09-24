package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tolerance/internal/proofs"
)

func fixtureRepo(t *testing.T) (nextTask, []byte) {
	t.Helper()
	tasks, err := proofs.LoadCatalog(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatal(err)
	}
	task := tasks[0]
	return nextTask{ProofID: "proof_x", Task: taskInfo{Slug: task.Slug, TaskMD: task.TaskMD, AgentTimeoutS: 5, RepoSHA256: task.RepoSHA256}}, task.RepoTar
}

func TestRunTask_ProducesDiffOfAgentChanges(t *testing.T) {
	task, repo := fixtureRepo(t)
	res, err := runTask(context.Background(), task, repo, `test -f TASK.md && printf 'package retry\n' > extra.go && sed -i.bak 's/Base \* time.Duration(attempt)/Base << uint(attempt-1)/' retry.go && rm retry.go.bak && echo done && echo "token=SUPERSECRET1234567" >&2`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ExitCode != 0 || !strings.Contains(res.Diff, "+++ b/extra.go") || !strings.Contains(res.Diff, "Base << uint(attempt-1)") {
		t.Fatalf("unexpected result: exit=%d diff=%q", res.ExitCode, res.Diff)
	}
	if strings.Contains(res.Diff, "TASK.md") {
		t.Fatalf("TASK.md must not be part of the diff")
	}
	if !strings.Contains(res.LogTail, "done") || strings.Contains(res.LogTail, "SUPERSECRET1234567") {
		t.Fatalf("log tail not sanitized or missing: %q", res.LogTail)
	}
	if res.DurationMS <= 0 {
		t.Fatalf("duration must be measured")
	}
}

func TestRunTask_TimesOutAndRejectsBadTarball(t *testing.T) {
	task, repo := fixtureRepo(t)
	task.Task.AgentTimeoutS = 1
	start := time.Now()
	res, err := runTask(context.Background(), task, repo, "sleep 10")
	if err != nil || !res.TimedOut || time.Since(start) > 5*time.Second {
		t.Fatalf("expected timeout, got err=%v res=%+v", err, res)
	}
	task.Task.RepoSHA256 = "deadbeef"
	if _, err := runTask(context.Background(), task, repo, "true"); err == nil {
		t.Fatalf("sha mismatch must be an error")
	}
}

func TestRunTask_BinaryFilesAndLargeFiles(t *testing.T) {
	task, repo := fixtureRepo(t)
	// A small binary file travels as a git binary patch; a file over 1 MiB
	// stays behind.
	res, err := runTask(context.Background(), task, repo,
		`printf 'a\000b\001c' > blob.bin && head -c 2000000 /dev/zero > big.dat && printf 'package retry\n' > extra.go`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(res.Diff, "Binary files") {
		t.Fatalf("binary change must be a real patch, got placeholder:\n%s", res.Diff)
	}
	if !strings.Contains(res.Diff, "GIT binary patch") || !strings.Contains(res.Diff, "b/blob.bin") {
		t.Fatalf("binary patch missing:\n%s", res.Diff)
	}
	if strings.Contains(res.Diff, "big.dat") {
		t.Fatalf("files over 1 MiB must be left out of the diff")
	}
	if !strings.Contains(res.Diff, "+++ b/extra.go") {
		t.Fatalf("ordinary changes must still be there:\n%s", res.Diff)
	}

	// The diff must apply to a clean copy, as the worker will do.
	dir := t.TempDir()
	if err := proofs.Untar(repo, dir); err != nil {
		t.Fatal(err)
	}
	apply := exec.Command("git", "apply", "-")
	apply.Dir, apply.Stdin = dir, strings.NewReader(res.Diff)
	if out, err := apply.CombinedOutput(); err != nil {
		t.Fatalf("diff does not apply: %v\n%s", err, out)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "blob.bin")); string(b) != "a\x00b\x01c" {
		t.Fatalf("blob.bin = %q", b)
	}
}

func TestRunTask_TimeoutKillsTheWholeProcessGroup(t *testing.T) {
	task, repo := fixtureRepo(t)
	task.Task.AgentTimeoutS = 1
	marker := filepath.Join(t.TempDir(), "survived")
	// The subshell forks a grandchild that outlives sh; only a group kill
	// stops it before it writes the marker.
	start := time.Now()
	res, err := runTask(context.Background(), task, repo, `(sleep 2; touch `+marker+`) & sleep 100`)
	if err != nil || !res.TimedOut {
		t.Fatalf("expected timeout, got err=%v res=%+v", err, res)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("timeout took %s", time.Since(start))
	}
	time.Sleep(2500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("a process the agent started survived the timeout")
	}
}
