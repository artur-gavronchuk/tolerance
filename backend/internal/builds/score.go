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

// Checks is the score breakdown stored with an entry and shown on its card.
type Checks struct {
	Passed  int             `json:"passed"`
	Total   int             `json:"total"`
	Failed  []string        `json:"failed"`
	Points  map[string]int  `json:"points"` // scenarios (of 70), a11y, mobile, perf (of 10 each)
	Quality json.RawMessage `json:"quality,omitempty"`
}

// score turns a run into 0..100: 70 for the share of scenarios passed, 10 each for accessibility, mobile fit
// and a clean, light first load. A signal the run could not measure scores 0.
func score(scenarios int, r runReport, rawQuality json.RawMessage) (int, Checks) {
	c := Checks{Total: scenarios, Failed: []string{}, Points: map[string]int{}, Quality: rawQuality}
	for _, res := range r.Results {
		if res.Passed {
			c.Passed++
		} else if len(c.Failed) < 30 {
			c.Failed = append(c.Failed, res.Name)
		}
	}
	if scenarios > 0 {
		c.Points["scenarios"] = 70 * min(c.Passed, scenarios) / scenarios
	} else {
		c.Points["scenarios"] = 70
	}
	q := r.Quality
	if q.A11y != nil {
		worst := func(f func(impactCounts) int) int { return max(f(q.A11y.Desktop), f(q.A11y.Mobile)) }
		c.Points["a11y"] = clamp(10 - 4*worst(func(i impactCounts) int { return i.Critical }) -
			2*worst(func(i impactCounts) int { return i.Serious }) - worst(func(i impactCounts) int { return i.Moderate }))
	}
	if q.Mobile != nil && !q.Mobile.Overflow {
		c.Points["mobile"] = clamp(10 - min(q.Mobile.SmallTargets, 5))
	} else {
		c.Points["mobile"] = 0
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
	for _, k := range []string{"a11y", "perf"} {
		if _, ok := c.Points[k]; !ok {
			c.Points[k] = 0
		}
	}
	total := 0
	for _, v := range c.Points {
		total += v
	}
	return min(total, 100), c
}

func clamp(n int) int { return max(0, min(10, n)) }
