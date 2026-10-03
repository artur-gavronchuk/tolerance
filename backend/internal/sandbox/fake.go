package sandbox

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Fake returns a canned result; tests and ARENA_SANDBOX=fake use it.
type Fake struct {
	Result Result
	Err    error
	Calls  []Request
	// Optimize is returned by RunOptimize.
	Optimize      OptimizeResult
	OptimizeCalls []OptimizeRequest
}

func (f *Fake) Run(_ context.Context, req Request) (Result, error) {
	f.Calls = append(f.Calls, req)
	return f.Result, f.Err
}

// PassAll runs nothing and reports every top-level Test function in the work
// directory's *_test.go files as passed (for Request.Language "python":
// every top-level test_ function of test_*.py and *_test.py, named
// "path::function" with the path relative to WorkDir). ARENA_SANDBOX=fake uses it so a
// local stack without Docker still reaches a passed verdict, which requires
// every hidden test to appear in the result.
type PassAll struct{}

var (
	testFunc   = regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	pyTestFunc = regexp.MustCompile(`(?m)^def (test_\w+)\(`)
)

func isPyTestFile(base string) bool {
	return strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py"))
}

func (PassAll) Run(_ context.Context, req Request) (Result, error) {
	if req.Language == "python" {
		return passAllPython(req.WorkDir)
	}
	var res Result
	err := filepath.WalkDir(req.WorkDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return err
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range testFunc.FindAllStringSubmatch(string(body), -1) {
			if m[1] != "TestMain" {
				res.Tests = append(res.Tests, TestResult{Name: m[1], Passed: true})
			}
		}
		return nil
	})
	return res, err
}

func passAllPython(workDir string) (Result, error) {
	var res Result
	err := filepath.WalkDir(workDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !isPyTestFile(d.Name()) {
			return err
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(workDir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for _, m := range pyTestFunc.FindAllStringSubmatch(string(body), -1) {
			res.Tests = append(res.Tests, TestResult{Name: rel + "::" + m[1], Passed: true})
		}
		return nil
	})
	return res, err
}
