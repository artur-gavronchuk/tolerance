package sandbox_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
)

const fixture = "../../../fixtures/proofs/go-fix-retry"

func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		if os.Getenv("ARENA_TEST_REQUIRE_DOCKER") == "1" {
			t.Fatalf("docker unavailable: %v", err)
		}
		t.Skipf("docker unavailable: %v", err)
	}
	build := exec.Command("docker", "build", "-q", "-t", "arena-proof-go:1", fixture)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build image: %v\n%s", err, out)
	}
}

func prepare(t *testing.T, fix func(dir string)) string {
	t.Helper()
	dir := t.TempDir()
	tasks, err := proofs.LoadCatalog(filepath.Dir(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := proofs.Untar(tasks[0].RepoTar, dir); err != nil {
		t.Fatal(err)
	}
	fix(dir)
	if err := proofs.Untar(tasks[0].HiddenTar, dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

const fixedBackoff = `
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	if attempt > 20 {
		return Max
	}
	d := Base << uint(attempt-1)
	if d > Max {
		return Max
	}
	return d
}
`

func TestDocker_PassesFailsAndReportsMissingImage(t *testing.T) {
	requireDocker(t)
	r := sandbox.NewDocker()
	req := func(dir string) sandbox.Request {
		return sandbox.Request{WorkDir: dir, Image: "arena-proof-go:1", RunCmd: "go test ./... -json -count=1", Timeout: 2 * time.Minute}
	}

	good := prepare(t, func(dir string) {
		src, _ := os.ReadFile(filepath.Join(dir, "retry.go"))
		start := []byte("// Backoff returns the delay before attempt n (1-based).\n")
		i := bytesIndex(src, start)
		j := bytesIndex(src, []byte("// Do calls fn"))
		out := append(append(append([]byte{}, src[:i]...), []byte(fixedBackoff)...), src[j:]...)
		_ = os.WriteFile(filepath.Join(dir, "retry.go"), out, 0o644)
	})
	res, err := r.Run(context.Background(), req(good))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ExitCode != 0 || len(res.Tests) != 8 {
		t.Fatalf("expected 8 passing tests, got exit=%d tests=%+v\n%s", res.ExitCode, res.Tests, res.Output)
	}
	for _, tr := range res.Tests {
		if !tr.Passed {
			t.Fatalf("%s failed with the reference fix", tr.Name)
		}
	}

	bad := prepare(t, func(string) {})
	res, err = r.Run(context.Background(), req(bad))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	failed := 0
	for _, tr := range res.Tests {
		if !tr.Passed {
			failed++
		}
	}
	if res.ExitCode == 0 || failed != 3 {
		t.Fatalf("expected 3 failing tests on the unfixed repo, got exit=%d failed=%d", res.ExitCode, failed)
	}

	slow := prepare(t, func(string) {})
	res, err = r.Run(context.Background(), sandbox.Request{WorkDir: slow, Image: "arena-proof-go:1", RunCmd: "sleep 30", Timeout: 3 * time.Second})
	if err != nil || !res.TimedOut {
		t.Fatalf("expected timeout, got err=%v res=%+v", err, res)
	}

	_, err = r.Run(context.Background(), sandbox.Request{WorkDir: bad, Image: "arena-proof-does-not-exist:0", RunCmd: "true", Timeout: 30 * time.Second})
	if err == nil {
		t.Fatalf("missing image must be an infra error")
	}
}

func bytesIndex(b, sub []byte) int {
	for i := 0; i+len(sub) <= len(b); i++ {
		if string(b[i:i+len(sub)]) == string(sub) {
			return i
		}
	}
	return -1
}
