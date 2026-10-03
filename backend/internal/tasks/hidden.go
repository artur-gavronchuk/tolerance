package tasks

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

// IsPyTestFile reports whether a base name is a pytest module the way hidden tests are named.
func IsPyTestFile(base string) bool {
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
		return nil, fmt.Errorf("tasks: hidden tests: unsupported language %q", language)
	}
	gz, err := gzip.NewReader(bytes.NewReader(hiddenTar))
	if err != nil {
		return nil, fmt.Errorf("tasks: hidden tests: %w", err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tasks: hidden tests: %w", err)
		}
		name := strings.TrimPrefix(h.Name, "./")
		base := path.Base(name)
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if language == "go" && !strings.HasSuffix(base, ".go") || language == "python" && !IsPyTestFile(base) {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(tr, 16<<20))
		if err != nil {
			return nil, fmt.Errorf("tasks: hidden tests: %w", err)
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
