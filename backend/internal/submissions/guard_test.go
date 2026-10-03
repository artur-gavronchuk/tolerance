package submissions

import "testing"

func TestTestFileTouched(t *testing.T) {
	cases := []struct {
		lang, diff string
		want       bool
	}{
		{"go", "diff --git a/retry_test.go b/retry_test.go\n--- a/retry_test.go\n+++ b/retry_test.go\n", true},
		{"go", "diff --git a/retry.go b/retry.go\n--- a/retry.go\n+++ b/retry.go\n", false},
		{"python", "diff --git a/test_mine.py b/test_mine.py\nnew file mode 100644\n--- /dev/null\n+++ b/test_mine.py\n", true},
		{"python", "diff --git a/pkg/limiter_test.py b/pkg/limiter_test.py\n", true},
		{"python", "diff --git a/conftest.py b/conftest.py\n--- /dev/null\n+++ b/conftest.py\n", true},
		{"python", "diff --git a/pyproject.toml b/pyproject.toml\n", true},
		{"python", "diff --git a/sitecustomize.py b/sitecustomize.py\n", true},
		{"python", "diff --git a/evil.pth b/evil.pth\n", true},
		{"python", "rename from limiter.py\nrename to test_limiter.py\n", true},
		{"python", "diff --git \"a/my dir/test_x.py\" \"b/my dir/test_x.py\"\n", true},
		{"python", "diff --git a/limiter.py b/limiter.py\n--- a/limiter.py\n+++ b/limiter.py\n@@ -1 +1 @@\n-# see test_old.py\n+# conftest.py is mentioned in a comment\n", false},
		{"python", "diff --git a/latest.py b/latest.py\n", false},
		{"python", "diff --git a/.pytest.ini b/.pytest.ini\n", true},
		{"python", "diff --git a/forcepass-1.0.dist-info/METADATA b/forcepass-1.0.dist-info/METADATA\n", true},
		{"python", "--- /dev/null\n+++ b/x.egg-info/entry_points.txt\n", true},
		{"python", "diff --git a/pytest.py b/pytest.py\n", true},
		{"python", "diff --git a/sub/pytest.py b/sub/pytest.py\n", true},
		{"python", "diff --git a/_pytest/reports.py b/_pytest/reports.py\n", true},
		{"python", "diff --git a/vendor/pluggy/_manager.py b/vendor/pluggy/_manager.py\n", true},
		{"python", "diff --git a/pytest_utils.py b/pytest_utils.py\n", false},
	}
	for _, c := range cases {
		if got := TestFileTouched(c.lang, c.diff); got != c.want {
			t.Errorf("%s %q: got %v", c.lang, c.diff, got)
		}
	}
}

func TestHarnessTampered(t *testing.T) {
	py := func(file, added string) string {
		return "diff --git a/" + file + " b/" + file + "\n--- a/" + file + "\n+++ b/" + file + "\n@@ -1 +1 @@\n-x\n+" + added + "\n"
	}
	cases := []struct {
		lang, diff string
		want       bool
	}{
		{"python", py("intervals.py", "import _pytest.reports"), true},
		{"python", py("pkg/m.py", "from pluggy import HookimplMarker"), true},
		{"python", py("m.py", "import pytest"), true},
		{"python", py("m.py", "return sorted(ranges)"), false},
		// only added lines count
		{"python", "diff --git a/m.py b/m.py\n--- a/m.py\n+++ b/m.py\n@@ -1 +1 @@\n-import pytest\n+pass\n", false},
		// a .py file named like a test is a test file: guarded elsewhere
		{"python", py("test_x.py", "import pytest"), false},
		{"go", py("lru.go", `import "testing"`), true},
		{"go", py("lru.go", "\t\"testing\""), true},
		{"go", py("lru_test.go", `import "testing"`), false},
		{"go", py("lru.go", "return x"), false},
		{"go", py("lru.go", `import "sort"`), false},
	}
	for _, c := range cases {
		if got := HarnessTampered(c.lang, c.diff); got != c.want {
			t.Errorf("%s %q: got %v", c.lang, c.diff, got)
		}
	}
}
