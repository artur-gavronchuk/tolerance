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
}

func (f *Fake) Run(_ context.Context, req Request) (Result, error) {
	f.Calls = append(f.Calls, req)
	return f.Result, f.Err
}

// PassAll runs nothing and reports every top-level Test function in the work
// directory's *_test.go files as passed. ARENA_SANDBOX=fake uses it so a
// local stack without Docker still reaches a passed verdict, which requires
// every hidden test to appear in the result.
type PassAll struct{}

var testFunc = regexp.MustCompile(`(?m)^func (Test\w+)\(`)

func (PassAll) Run(_ context.Context, req Request) (Result, error) {
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
