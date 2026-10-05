package builds

import "encoding/json"

// runReport is what runner.py prints after @@BUILD@@.
type runReport struct {
	Results []struct {
		Name   string `json:"name"`
		Passed bool   `json:"passed"`
	} `json:"results"`
	Quality quality `json:"quality"`
}

type impactCounts struct {
	Critical int `json:"critical"`
	Serious  int `json:"serious"`
	Moderate int `json:"moderate"`
	Minor    int `json:"minor"`
}

type quality struct {
	A11y *struct {
		Desktop  impactCounts `json:"desktop"`
		Mobile   impactCounts `json:"mobile"`
		TopRules []string     `json:"top_rules"`
	} `json:"a11y,omitempty"`
	Mobile *struct {
		Overflow     bool `json:"overflow"`
		SmallTargets int  `json:"small_targets"`
	} `json:"mobile,omitempty"`
	Perf *struct {
		Bytes    int `json:"bytes"`
		Requests int `json:"requests"`
		LoadMS   int `json:"load_ms"`
		Errors   int `json:"errors"`
	} `json:"perf,omitempty"`
}

// Checks is what the run found, stored with an entry. Points holds the quality hints (a11y, mobile, perf, of
// 10 each): shown on the entry, not part of the score.
type Checks struct {
	Passed  int             `json:"passed"`
	Total   int             `json:"total"`
	Failed  []string        `json:"failed"` // hidden from everyone but the owner while the challenge is open
	Points  map[string]int  `json:"points"`
	Quality json.RawMessage `json:"quality,omitempty"`
}

// score turns a run into the test points: TestPoints for the share of hidden tests passed (rounded down).
func score(tests int, r runReport, rawQuality json.RawMessage) (int, Checks) {
	c := Checks{Total: tests, Failed: []string{}, Points: map[string]int{}, Quality: rawQuality}
	for _, res := range r.Results {
		if res.Passed {
			c.Passed++
		} else if len(c.Failed) < 50 {
			c.Failed = append(c.Failed, res.Name)
		}
	}
	pts := 0
	if tests > 0 {
		pts = TestPoints * min(c.Passed, tests) / tests
	} else {
		pts = TestPoints // nothing to fail (the fake sandbox without tests)
	}
	q := r.Quality
	if q.A11y != nil {
		worst := func(f func(impactCounts) int) int { return max(f(q.A11y.Desktop), f(q.A11y.Mobile)) }
		c.Points["a11y"] = clamp(10 - 4*worst(func(i impactCounts) int { return i.Critical }) -
			2*worst(func(i impactCounts) int { return i.Serious }) - worst(func(i impactCounts) int { return i.Moderate }))
	}
	if q.Mobile != nil && !q.Mobile.Overflow {
		c.Points["mobile"] = clamp(10 - min(q.Mobile.SmallTargets, 5))
	}
	if q.Perf != nil {
		p := 10
		if q.Perf.Errors > 0 {
			p -= 5
		}
		switch {
		case q.Perf.LoadMS > 3000:
			p -= 4
		case q.Perf.LoadMS > 1500:
			p -= 2
		}
		switch {
		case q.Perf.Bytes > 3<<20:
			p -= 3
		case q.Perf.Bytes > 1<<20:
			p -= 2
		}
		c.Points["perf"] = clamp(p)
	}
	return pts, c
}

func clamp(n int) int { return max(0, min(10, n)) }
