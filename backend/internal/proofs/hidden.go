package proofs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

var (
	goTestFunc = regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	pyTestFunc = regexp.MustCompile(`(?m)^def (test_\w+)\(`)
)

func isPyTestFile(base string) bool {
	return strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py"))
}

// HiddenTestNames lists the tests in a hidden-tests tarball the way the
// sandbox reports them: top-level Test functions for Go, "path::test_name"
// for pytest (top-level functions of test_*.py and *_test.py only; hidden
// Python tests are written that way). The tarball comes from the server's
// catalog and is never touched by the participant, so these names are what
// a verdict and a score may count.
func HiddenTestNames(language string, hiddenTar []byte) ([]string, error) {
	if language != "go" && language != "python" {
		return nil, fmt.Errorf("proofs: hidden tests: unsupported language %q", language)
	}
	gz, err := gzip.NewReader(bytes.NewReader(hiddenTar))
	if err != nil {
		return nil, fmt.Errorf("proofs: hidden tests: %w", err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("proofs: hidden tests: %w", err)
		}
		name := strings.TrimPrefix(h.Name, "./")
		base := path.Base(name)
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if language == "go" && !strings.HasSuffix(base, ".go") || language == "python" && !isPyTestFile(base) {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(tr, 16<<20))
		if err != nil {
			return nil, fmt.Errorf("proofs: hidden tests: %w", err)
		}
		if language == "go" {
			for _, m := range goTestFunc.FindAllStringSubmatch(string(body), -1) {
				if m[1] != "TestMain" {
					names = append(names, m[1])
				}
			}
			continue
		}
		for _, m := range pyTestFunc.FindAllStringSubmatch(string(body), -1) {
			names = append(names, name+"::"+m[1])
		}
	}
	return names, nil
}

// testFileHeaders are the patch lines that name a file the patch touches.
var testFileHeaders = []string{"diff --git ", "--- ", "+++ ", "rename from ", "rename to ", "copy from ", "copy to "}

// pyGuarded are files that change how pytest collects or reports tests, or
// run code before pytest does.
var pyGuarded = map[string]bool{"conftest.py": true, "pytest.ini": true, "tox.ini": true, "setup.cfg": true,
	"pyproject.toml": true, "sitecustomize.py": true, "usercustomize.py": true}

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
				if pyGuarded[base] || strings.HasSuffix(base, ".pth") || isPyTestFile(base) {
					return true
				}
			}
		}
	}
	return false
}
