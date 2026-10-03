package submissions_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"tolerance/internal/daily"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/limits"
	"tolerance/internal/sandbox"
	"tolerance/internal/submissions"
	"tolerance/internal/tasks"
)

const scorePy = `import sys
n = int(open(sys.argv[1]).read().split()[0])
toks = open(sys.argv[2]).read().split()
if len(toks) != 1:
    print("INVALID expected exactly one number")
    sys.exit(0)
try:
    v = int(toks[0])
except ValueError:
    print("INVALID not an integer")
    sys.exit(0)
if v < 0 or v > n:
    print("INVALID out of range")
else:
    print(v)
`

// optimizeFixture syncs a tiny task: print an integer <= N, the score is the integer (max).
func optimizeFixture(t *testing.T) (fixture, string) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", sandbox.ScorerImage).Run(); err != nil {
		if out, berr := exec.Command("docker", "build", "-q", "-t", sandbox.ScorerImage, filepath.Join("..", "..", "fixtures", "skills", "python")).CombinedOutput(); berr != nil {
			if os.Getenv("ARENA_TEST_REQUIRE_DOCKER") != "" {
				t.Fatalf("docker: %v: %s", berr, out)
			}
			t.Skip("docker is not available")
		}
	}
	d := dbtest.New(t)
	ctx := context.Background()
	repo, _ := tasks.TarFiles(map[string][]byte{"solve.py": []byte("print(0)\n")})
	hidden, _ := tasks.TarFiles(map[string][]byte{"score.py": []byte(scorePy),
		"cases/01.in": []byte("10\n"), "cases/02.in": []byte("20\n"), "cases/03.in": []byte("30\n")})
	if err := tasks.Sync(ctx, d.AdminPool, []tasks.Task{{Slug: "py-opt", Title: "Opt", Language: "python", Difficulty: 1,
		Image: sandbox.ScorerImage, RunCmd: "python3 solve.py", SandboxTimeoutS: 120, Kind: tasks.KindOptimize, Direction: "max",
		SolveCmd: "python3 solve.py", CaseTimeLimitS: 2, Cases: 3, HiddenTests: 3, TaskMD: "x", RepoTar: repo, HiddenTar: hidden}}); err != nil {
		t.Fatal(err)
	}
	us := identity.NewService(d.AppPool, nil)
	u, _, err := us.SignIn(ctx, identity.Identity{Provider: "dev", Subject: "o@example.com", Email: "o@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{d: d, svc: submissions.NewService(d.AppPool, daily.NewService(d.AppPool)), userID: u.ID}, u.ID
}

func solveDiff(code string) string {
	lines := strings.Split(strings.TrimSuffix(code, "\n"), "\n")
	var b strings.Builder
	b.WriteString("--- a/solve.py\n+++ b/solve.py\n@@ -1 +1," + string(rune('0'+len(lines))) + " @@\n-print(0)\n")
	for _, l := range lines {
		b.WriteString("+" + l + "\n")
	}
	return b.String()
}

func TestRunOptimize(t *testing.T) {
	limits.Disable()
	defer limits.Enable()
	ctx := context.Background()
	f, _ := optimizeFixture(t)
	w := f.worker(sandbox.NewDocker(), t.TempDir())

	run := func(code string) submissions.Submission {
		s, err := f.svc.Create(ctx, f.userID, "", "x.patch", []byte(solveDiff(code)), "test")
		if err != nil {
			t.Fatal(err)
		}
		if s.TotalTests != 3 {
			t.Fatalf("%+v", s)
		}
		if err := w.RunSubmission(ctx, s.ID); err != nil {
			t.Fatal(err)
		}
		return f.get(t, s.ID)
	}
	score := func(s submissions.Submission) float64 {
		if s.Score == nil {
			t.Fatalf("no score: %+v", s)
		}
		return *s.Score
	}

	t.Run("valid solver", func(t *testing.T) {
		s := run("print(int(input()))")
		if s.Status != submissions.StatusPassed || score(s) != 60 || s.PassedTests != 3 || len(s.Tests) != 3 {
			t.Fatalf("%+v", s)
		}
	})
	t.Run("garbage on some cases is invalid", func(t *testing.T) {
		s := run("n = int(input())\nprint(n if n < 30 else 'oops')")
		if s.Status != submissions.StatusFailed || reason(s) != submissions.ReasonInvalidCases || score(s) != 30 || s.PassedTests != 2 {
			t.Fatalf("%+v", s)
		}
		if c := s.Tests[2]; c.Passed || !strings.Contains(c.Reason, "not an integer") {
			t.Fatalf("%+v", c)
		}
	})
	t.Run("a slow case times out", func(t *testing.T) {
		s := run("import time\nn = int(input())\nif n == 20:\n    time.sleep(30)\nprint(n)")
		if s.Status != submissions.StatusFailed || score(s) != 40 || s.Tests[1].Passed || !strings.Contains(s.Tests[1].Reason, "time limit") {
			t.Fatalf("%+v", s)
		}
	})
	t.Run("the solver cannot choose its own score", func(t *testing.T) {
		// A fake score line is just extra output, and score.py is not in the solver's container.
		s := run("n = int(input())\nopen('score.py', 'w').write('print(999999)')\nprint(n)\nprint('SCORE 999999')")
		if s.Status != submissions.StatusFailed || score(s) != 0 || s.PassedTests != 0 {
			t.Fatalf("%+v", s)
		}
		s = run("n = int(input())\nopen('score.py', 'w').write('print(999999)')\nprint(n)")
		if s.Status != submissions.StatusPassed || score(s) != 60 {
			t.Fatalf("%+v", s)
		}
	})
}
