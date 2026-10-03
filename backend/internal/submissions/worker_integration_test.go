package submissions_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tolerance/internal/daily"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/limits"
	"tolerance/internal/sandbox"
	"tolerance/internal/submissions"
	"tolerance/internal/tasks"
)

const goodDiff = `--- a/retry.go
+++ b/retry.go
@@ -19,7 +19,14 @@ func Backoff(attempt int) time.Duration {
 	if attempt < 1 {
 		return 0
 	}
-	return Base * time.Duration(attempt)
+	if attempt > 20 {
+		return Max
+	}
+	d := Base << uint(attempt-1)
+	if d > Max {
+		return Max
+	}
+	return d
 }

 // Do calls fn until it succeeds or maxAttempts is used up.
`

// hiddenNames are the test functions in fixtures/proofs/go-fix-retry/_hidden.
var hiddenNames = []string{"TestHidden_BackoffSequence", "TestHidden_BackoffCapsAtMax", "TestHidden_BackoffZeroAndNegative",
	"TestHidden_DoStopsAtMaxAttempts", "TestHidden_DoReturnsContextErrorWhileWaiting"}

func passing(extra ...sandbox.TestResult) []sandbox.TestResult {
	out := append([]sandbox.TestResult{}, extra...)
	for _, n := range hiddenNames {
		out = append(out, sandbox.TestResult{Name: n, Passed: true})
	}
	return out
}

type fixture struct {
	d      *dbtest.DB
	svc    *submissions.Service
	userID string
}

func setup(t *testing.T) fixture {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	ts, err := tasks.LoadFlat(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.Sync(ctx, d.AdminPool, ts); err != nil {
		t.Fatal(err)
	}
	us := identity.NewService(d.AppPool, nil)
	u, _, err := us.SignIn(ctx, identity.Identity{Provider: "dev", Subject: "o@example.com", Email: "o@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{d: d, svc: submissions.NewService(d.AppPool, daily.NewService(d.AppPool)), userID: u.ID}
}

func (f fixture) submit(t *testing.T, diff string) string {
	t.Helper()
	s, err := f.svc.Create(context.Background(), f.userID, "", "fix.patch", []byte(diff), "test")
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != submissions.StatusQueued || s.Day == nil || s.TotalTests != 5 {
		t.Fatalf("%+v", s)
	}
	return s.ID
}

func (f fixture) worker(r sandbox.Runner, workDir string) *submissions.Worker {
	return submissions.NewWorker(f.d.AppPool, r, workDir, slog.New(slog.NewTextHandler(os.Stderr, nil)))
}

func (f fixture) get(t *testing.T, id string) submissions.Submission {
	t.Helper()
	s, err := f.svc.Get(context.Background(), f.userID, id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func reason(s submissions.Submission) string {
	if s.FailureReason == nil {
		return ""
	}
	return *s.FailureReason
}

func TestRunSubmission_Verdicts(t *testing.T) {
	ctx := context.Background()

	t.Run("all tests pass", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing(sandbox.TestResult{Name: "TestA", Passed: true})}}
		workDir := t.TempDir()
		id := f.submit(t, goodDiff)
		if err := f.worker(fake, workDir).RunSubmission(ctx, id); err != nil {
			t.Fatal(err)
		}
		s := f.get(t, id)
		if s.Status != submissions.StatusPassed || s.PassedTests != 5 || len(s.Tests) != 5 || s.FinishedAt == nil || reason(s) != "" {
			t.Fatalf("%+v", s)
		}
		if len(fake.Calls) != 1 || fake.Calls[0].Image != "arena-proof-go:1" {
			t.Fatalf("runner not called with the task image: %+v", fake.Calls)
		}
		if _, err := os.Stat(fake.Calls[0].WorkDir); err == nil {
			t.Fatalf("work dir must be removed after the run")
		}
	})

	t.Run("a hidden test fails", func(t *testing.T) {
		f := setup(t)
		tests := passing()[1:]
		tests = append(tests, sandbox.TestResult{Name: hiddenNames[0], Passed: false})
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 1, Tests: tests}}
		id := f.submit(t, goodDiff)
		_ = f.worker(fake, t.TempDir()).RunSubmission(ctx, id)
		s := f.get(t, id)
		if s.Status != submissions.StatusFailed || reason(s) != submissions.ReasonTestsFailed || s.PassedTests != 4 {
			t.Fatalf("%+v", s)
		}
	})

	t.Run("diff does not apply", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{}
		id := f.submit(t, "--- a/nope.go\n+++ b/nope.go\n@@ -1 +1 @@\n-x\n+y\n")
		_ = f.worker(fake, t.TempDir()).RunSubmission(ctx, id)
		s := f.get(t, id)
		if s.Status != submissions.StatusFailed || reason(s) != submissions.ReasonDoesNotApply || len(fake.Calls) != 0 {
			t.Fatalf("%+v calls=%d", s, len(fake.Calls))
		}
	})

	t.Run("a diff escaping the work dir is refused", func(t *testing.T) {
		limits.Disable() // four uploads here, more than the daily attempts
		defer limits.Enable()
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing()}}
		workDir := t.TempDir()
		w := f.worker(fake, workDir)
		escapes := map[string]string{
			"dotdot":  "diff --git a/../escape.txt b/../escape.txt\nnew file mode 100644\n--- /dev/null\n+++ b/../escape.txt\n@@ -0,0 +1 @@\n+pwned\n",
			"plain":   "--- /dev/null\n+++ b/../../escape.txt\n@@ -0,0 +1 @@\n+pwned\n",
			"git":     "diff --git a/.git/config b/.git/config\nnew file mode 100644\n--- /dev/null\n+++ b/.git/config\n@@ -0,0 +1 @@\n+pwned\n",
			"symlink": "diff --git a/link b/link\nnew file mode 120000\n--- /dev/null\n+++ b/link\n@@ -0,0 +1 @@\n+" + workDir + "\n\\ No newline at end of file\n",
		}
		for name, diff := range escapes {
			id := f.submit(t, diff)
			if err := w.RunSubmission(ctx, id); err != nil {
				t.Fatal(err)
			}
			s := f.get(t, id)
			if s.Status != submissions.StatusFailed || reason(s) != submissions.ReasonDoesNotApply {
				t.Fatalf("%s: %+v", name, s)
			}
		}
		if len(fake.Calls) != 0 {
			t.Fatalf("sandbox must not run: %d calls", len(fake.Calls))
		}
		for _, p := range []string{filepath.Join(workDir, "escape.txt"), filepath.Join(filepath.Dir(workDir), "escape.txt")} {
			if _, err := os.Stat(p); err == nil {
				t.Fatalf("patch wrote outside the work dir: %s", p)
			}
		}
	})

	t.Run("hidden tests that did not run fail the submission", func(t *testing.T) {
		f := setup(t)
		// Everything that ran passed, but only the visible tests ran.
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: []sandbox.TestResult{
			{Name: "TestDo_SucceedsFirstTry", Passed: true}, {Name: "TestBackoff_Doubles", Passed: true}}}}
		id := f.submit(t, goodDiff)
		if err := f.worker(fake, t.TempDir()).RunSubmission(ctx, id); err != nil {
			t.Fatal(err)
		}
		if s := f.get(t, id); s.Status != submissions.StatusFailed || reason(s) != submissions.ReasonTestsNotRun || s.PassedTests != 0 {
			t.Fatalf("%+v", s)
		}
		// One hidden test missing is enough.
		fake2 := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing()[1:]}}
		id2 := f.submit(t, goodDiff)
		_ = f.worker(fake2, t.TempDir()).RunSubmission(ctx, id2)
		if s := f.get(t, id2); s.Status != submissions.StatusFailed || reason(s) != submissions.ReasonTestsNotRun || s.PassedTests != 4 {
			t.Fatalf("%+v", s)
		}
	})

	t.Run("a diff touching a test file is refused", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing()}}
		testMain := "diff --git a/main_test.go b/main_test.go\nnew file mode 100644\n--- /dev/null\n+++ b/main_test.go\n@@ -0,0 +1,3 @@\n+package retry\n+\n+// TestMain here\n"
		id := f.submit(t, goodDiff+testMain)
		if err := f.worker(fake, t.TempDir()).RunSubmission(ctx, id); err != nil {
			t.Fatal(err)
		}
		if s := f.get(t, id); s.Status != submissions.StatusFailed || reason(s) != submissions.ReasonTestFiles || len(fake.Calls) != 0 {
			t.Fatalf("%+v calls=%d", s, len(fake.Calls))
		}
	})

	t.Run("importing testing into non-test code is harness tampering", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing()}}
		tamper := "diff --git a/extra.go b/extra.go\nnew file mode 100644\n--- /dev/null\n+++ b/extra.go\n@@ -0,0 +1,3 @@\n+package retry\n+\n+import \"testing\"\n"
		id := f.submit(t, goodDiff+tamper)
		if err := f.worker(fake, t.TempDir()).RunSubmission(ctx, id); err != nil {
			t.Fatal(err)
		}
		if s := f.get(t, id); s.Status != submissions.StatusFailed || reason(s) != submissions.ReasonHarness || len(fake.Calls) != 0 {
			t.Fatalf("%+v calls=%d", s, len(fake.Calls))
		}
	})

	t.Run("CRLF diff still applies", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing()}}
		id := f.submit(t, strings.ReplaceAll(goodDiff, "\n", "\r\n"))
		_ = f.worker(fake, t.TempDir()).RunSubmission(ctx, id)
		if s := f.get(t, id); s.Status != submissions.StatusPassed {
			t.Fatalf("CRLF diff: %+v", s)
		}
	})

	t.Run("sandbox timeout is a failure", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Result: sandbox.Result{TimedOut: true, ExitCode: -1}}
		id := f.submit(t, goodDiff)
		_ = f.worker(fake, t.TempDir()).RunSubmission(ctx, id)
		if s := f.get(t, id); s.Status != submissions.StatusFailed || reason(s) != submissions.ReasonTimeout {
			t.Fatalf("%+v", s)
		}
	})

	t.Run("runner error is infra_error and does not use an attempt", func(t *testing.T) {
		f := setup(t)
		fake := &sandbox.Fake{Err: errors.New("no docker")}
		w := f.worker(fake, t.TempDir())
		id := f.submit(t, goodDiff)
		if err := w.RunSubmission(ctx, id); err == nil {
			t.Fatalf("runner error must propagate so the job retries")
		}
		if s := f.get(t, id); s.Status != submissions.StatusRunning {
			t.Fatalf("before giving up the submission stays running: %+v", s)
		}
		if err := w.MarkInfraError(ctx, id, "no docker"); err != nil {
			t.Fatal(err)
		}
		if s := f.get(t, id); s.Status != submissions.StatusInfraError || s.FinishedAt == nil {
			t.Fatalf("%+v", s)
		}
		my, err := f.svc.MyDay(ctx, f.userID, daily.Today())
		if err != nil || my.(submissions.My).AttemptsUsed != 0 {
			t.Fatalf("infra_error must not count as an attempt: %+v %v", my, err)
		}
	})
}

func TestAttemptsAndPractice(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// limits are on in tests: three graded attempts, the fourth is refused.
	for i := 0; i < 3; i++ {
		f.submit(t, goodDiff)
	}
	_, err := f.svc.Create(ctx, f.userID, "", "fix.patch", []byte(goodDiff), "")
	var p *httpx.Problem
	if !errors.As(err, &p) || p.Status != 429 || p.Code != "attempts_exhausted" {
		t.Fatalf("fourth attempt: %v", err)
	}
	// An unrelated empty upload is rejected before it is counted.
	if _, err := f.svc.Create(ctx, f.userID, "", "x.patch", []byte("  \n"), ""); !errors.As(err, &p) || p.Code != "invalid_upload" {
		t.Fatalf("empty patch: %v", err)
	}
}

func repoFiles(t *testing.T, f fixture) map[string][]byte {
	t.Helper()
	var tarball []byte
	err := f.d.AdminPool.Raw().QueryRow(context.Background(), `SELECT repo_tar FROM tasks WHERE slug = 'go-fix-retry'`).Scan(&tarball)
	if err != nil {
		t.Fatal(err)
	}
	files, err := tasks.ReadTar(tarball)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func makeZip(t *testing.T, prefix string, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(prefix + name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}
	// junk that must be ignored
	for _, junk := range []string{"__MACOSX/._retry.go", ".DS_Store", ".git/HEAD", "__pycache__/x.pyc"} {
		w, _ := zw.Create(prefix + junk)
		_, _ = w.Write([]byte("junk"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestZipUploadBecomesApplyingDiff(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	files := repoFiles(t, f)
	orig := string(files["retry.go"])
	fixed := strings.Replace(orig, "return Base * time.Duration(attempt)", "d := Base << uint(attempt-1)\n\tif d > Max || attempt > 20 {\n\t\treturn Max\n\t}\n\treturn d", 1)
	if fixed == orig {
		t.Fatal("test fixture: bug line not found")
	}
	files["retry.go"] = []byte(fixed)
	// The downloaded zip carries TASK.md (tasks.RepoZip); an edited copy never reaches the diff.
	files["TASK.md"] = []byte("my agent's notes")

	// Both a zip with the files at the root and one wrapped in a single folder produce the same applying diff.
	for _, prefix := range []string{"", "retry-task/"} {
		fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: passing()}}
		s, err := f.svc.Create(ctx, f.userID, "", "result.zip", makeZip(t, prefix, files), "zip")
		if err != nil {
			t.Fatalf("prefix %q: %v", prefix, err)
		}
		if err := f.worker(fake, t.TempDir()).RunSubmission(ctx, s.ID); err != nil {
			t.Fatal(err)
		}
		if got := f.get(t, s.ID); got.Status != submissions.StatusPassed {
			t.Fatalf("prefix %q: zip diff did not apply: %+v", prefix, got)
		}
	}

	// An unchanged repo is an invalid upload, even with TASK.md edited.
	files["retry.go"] = []byte(orig)
	var p *httpx.Problem
	if _, err := f.svc.Create(ctx, f.userID, "", "same.zip", makeZip(t, "", files), ""); !errors.As(err, &p) || p.Code != "invalid_upload" {
		t.Fatalf("unchanged zip: %v", err)
	}

	// Unsafe archives are refused.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("../evil.go")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()
	if _, err := f.svc.Create(ctx, f.userID, "", "evil.zip", buf.Bytes(), ""); !errors.As(err, &p) || p.Code != "invalid_upload" {
		t.Fatalf("zip with ..: %v", err)
	}
}
