package sandbox

import (
	"bufio"
	"bytes"
	"encoding/json"
)

type goTestEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

// ParseGoTestJSON extracts per-test pass/fail from `go test -json` output.
// Package-level events (no Test) and non-JSON lines are ignored; a build
// failure therefore yields zero tests, which the caller treats as failed.
func ParseGoTestJSON(out []byte) []TestResult {
	var res []TestResult
	seen := map[string]int{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var ev goTestEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil || ev.Test == "" {
			continue
		}
		if ev.Action != "pass" && ev.Action != "fail" {
			continue
		}
		if i, ok := seen[ev.Test]; ok {
			res[i].Passed = ev.Action == "pass"
			continue
		}
		seen[ev.Test] = len(res)
		res = append(res, TestResult{Name: ev.Test, Passed: ev.Action == "pass"})
	}
	return res
}
