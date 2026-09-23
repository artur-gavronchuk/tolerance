package main

import (
	"context"
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
