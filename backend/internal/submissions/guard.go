package submissions

import (
	"path"
	"regexp"
	"strings"

	"tolerance/internal/tasks"
)

// testFileHeaders are the patch lines that name a file the patch touches.
var testFileHeaders = []string{"diff --git ", "--- ", "+++ ", "rename from ", "rename to ", "copy from ", "copy to "}

// pyGuarded are files that change how pytest collects or reports tests, or
// run code before pytest does.
var pyGuarded = map[string]bool{"conftest.py": true, "pytest.ini": true, ".pytest.ini": true, "tox.ini": true, "setup.cfg": true,
	"pyproject.toml": true, "sitecustomize.py": true, "usercustomize.py": true}

// pyHarnessPath reports whether a patch path lies inside package metadata
// (*.dist-info, *.egg-info: entry points auto-load pytest plugins) or names
// pytest itself or its core dependency (pytest.py, _pytest/, pluggy/), at
// any depth: a copy of any of them in the work directory could shadow or
// extend the real harness.
func pyHarnessPath(f string) bool {
	parts := strings.Split(strings.Trim(f, `"`), "/")
	for i, seg := range parts {
		switch {
		case strings.HasSuffix(seg, ".dist-info"), strings.HasSuffix(seg, ".egg-info"):
			return true
		case seg == "_pytest", seg == "pluggy":
			return true
		case seg == "pytest.py" && i == len(parts)-1:
			return true
		}
	}
	return false
}

// TestFileTouched reports whether the patch adds, edits, deletes or renames
// a test file of the task's language. Participants fix the code, not the
// tests: for Go a *_test.go file (a TestMain could narrow what runs); for
// Python a test module, conftest.py or pytest's config (each can rewrite
// results), or a startup hook (sitecustomize.py, *.pth).
func TestFileTouched(language, diff string) bool {
	for _, line := range strings.Split(diff, "\n") {
		for _, prefix := range testFileHeaders {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			if language == "go" {
				if strings.Contains(line, "_test.go") {
					return true
				}
				continue
			}
			for _, f := range strings.Fields(line[len(prefix):]) {
				base := path.Base(strings.Trim(f, `"`))
				if pyGuarded[base] || strings.HasSuffix(base, ".pth") || tasks.IsPyTestFile(base) || pyHarnessPath(f) {
					return true
				}
			}
		}
	}
	return false
}

var pyHarnessRef = regexp.MustCompile(`pytest|pluggy`)

// HarnessTampered is a cheap deterrent against forging results from inside
// the process under test: for Python, an added line in a non-test .py file
// that mentions pytest, _pytest or pluggy; for Go, an added line in a
// non-test .go file that imports "testing". It is not a proof of honesty (an
// out-of-process harness is the real fix), only a tripwire for the obvious
// monkeypatch.
func HarnessTampered(language, diff string) bool {
	var file string
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			fields := strings.Fields(line)
			file = strings.TrimPrefix(strings.Trim(fields[len(fields)-1], `"`), "b/")
		case strings.HasPrefix(line, "+++ "):
			if p := strings.TrimSpace(line[4:]); p != "/dev/null" {
				file = strings.TrimPrefix(strings.Trim(p, `"`), "b/")
			}
		case strings.HasPrefix(line, "--- "):
		case strings.HasPrefix(line, "+"):
			base := path.Base(file)
			switch language {
			case "python":
				if strings.HasSuffix(base, ".py") && !tasks.IsPyTestFile(base) && pyHarnessRef.MatchString(line) {
					return true
				}
			case "go":
				if strings.HasSuffix(base, ".go") && !strings.HasSuffix(base, "_test.go") && strings.Contains(line, `"testing"`) {
					return true
				}
			}
		}
	}
	return false
}
