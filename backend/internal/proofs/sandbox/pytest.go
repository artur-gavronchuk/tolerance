package sandbox

import (
	"bufio"
	"bytes"
	"strings"
)

// ParsePytest reads the `-rA` short summary pytest prints at the end of a
// run: "PASSED path::name", "FAILED path::name - reason", "ERROR path::name
// - reason", and "ERROR path - reason" for a module that failed to import.
// Only lines after the first summary header count, so what a test prints
// into its captured output cannot pose as a result, and a test reported as
// failed anywhere after that header stays failed whatever else claims it
// passed. A module-level ERROR fails every test of that module the summary
// names. Code under test runs in the same process and could still forge
// output (the same holds for go test); this only closes the cheap tricks.
func ParsePytest(out []byte) []TestResult {
	var res []TestResult
	index := map[string]int{}
	var broken []string
	inSummary := false
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if !inSummary {
			inSummary = strings.HasPrefix(line, "=") && strings.Contains(line, " short test summary info ")
			continue
		}
		var passed bool
		var rest string
		switch {
		case strings.HasPrefix(line, "PASSED "):
			passed, rest = true, line[len("PASSED "):]
		case strings.HasPrefix(line, "FAILED "):
			rest = line[len("FAILED "):]
		case strings.HasPrefix(line, "ERROR "):
			rest = line[len("ERROR "):]
		default:
			continue
		}
		name := rest
		if i := strings.Index(rest, " - "); i >= 0 {
			name = rest[:i]
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !strings.Contains(name, "::") {
			if !passed {
				broken = append(broken, name+"::")
			}
			continue
		}
		if i, ok := index[name]; ok {
			res[i].Passed = res[i].Passed && passed
			continue
		}
		index[name] = len(res)
		res = append(res, TestResult{Name: name, Passed: passed})
	}
	for i := range res {
		for _, prefix := range broken {
			if strings.HasPrefix(res[i].Name, prefix) {
				res[i].Passed = false
			}
		}
	}
	return res
}
