package sandbox

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

// ParsePytest reads the `-rA` short summary pytest prints at the end of a
// run: "PASSED path::name", "FAILED path::name - reason", "ERROR path::name
// - reason", and "ERROR path - reason" for a module that failed to import.
// With -rA pytest also prints the captured stdout of passing tests before
// the real summary, so a test (or the code under test) can print a fake
// header and fake PASSED lines. Only the window between the LAST
// "short test summary info" header and the final stats line ("N passed in
// 0.03s") counts; everything before the last header or after the stats line
// is ignored, and a hidden test missing from the real summary is simply not
// passed. Within the window a test reported as failed stays failed whatever
// else claims it passed, and a module-level ERROR fails every test of that
// module the summary names. Code under test runs in the same process and
// could still forge output that comes last (monkeypatching _pytest's report
// classes, an atexit hook printing a complete fake summary). That is a known
// risk, mitigated by --show-capture=no in the skill's run_cmd and by the
// worker's harness_tampering check (proofs.HarnessTampered); an
// out-of-process harness is the follow-up. This parser closes the cheap tricks.
func ParsePytest(out []byte) []TestResult {
	var res []TestResult
	index := map[string]int{}
	var broken []string
	var lines []string
	last := -1
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "=") && strings.Contains(line, " short test summary info ") {
			last = len(lines)
		}
		lines = append(lines, line)
	}
	if last < 0 {
		return nil
	}
	for _, line := range lines[last+1:] {
		if isPytestStats(line) {
			break
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

// statsRe matches pytest's closing line: "3 passed in 0.02s",
// "1 failed, 2 passed, 2 errors in 0.03s (0:00:00)", optionally in "=== ===".
var statsRe = regexp.MustCompile(`^(=+ )?(\d+ [a-z ]+(, )?)+ in \d+(\.\d+)?s( \(.*\))?( =+)?$`)

func isPytestStats(line string) bool {
	return statsRe.MatchString(line)
}
