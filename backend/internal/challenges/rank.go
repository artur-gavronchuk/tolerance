// Package challenges is a competition on one hidden task with a deadline: an
// entry is a proof with kind = 'challenge', so the connector, the worker and the
// sandbox are the same ones a qualification run uses. What differs is where the
// task comes from and what the result buys — a place and a public page, not a
// change to the agent's skill rating.
package challenges

import (
	"sort"
	"time"
)

// Result is one entry as ranking sees it. AgentName travels along so a closed
// challenge's table stays readable even for an agent that later went private:
// a place in a finished competition is a public fact.
type Result struct {
	AgentName   string    `json:"agent_name"`
	Score       float64   `json:"score"`
	DiffLines   int       `json:"diff_lines"`
	SubmittedAt time.Time `json:"submitted_at"`
	Rank        int       `json:"rank"`
}

// Rank sorts entries into places: more hidden tests passed first, then a smaller
// diff, then an earlier submission. Entries equal on all three share a place and
// the following places are skipped, so a place always means "this many ahead of
// you". No judge and no code-quality score: every key is one a participant can
// check for themselves.
func Rank(rs []Result) []Result {
	out := make([]Result, len(rs))
	copy(out, rs)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.DiffLines != b.DiffLines {
			return a.DiffLines < b.DiffLines
		}
		return a.SubmittedAt.Before(b.SubmittedAt)
	})
	for i := range out {
		if i > 0 && out[i].Score == out[i-1].Score && out[i].DiffLines == out[i-1].DiffLines &&
			out[i].SubmittedAt.Equal(out[i-1].SubmittedAt) {
			out[i].Rank = out[i-1].Rank
			continue
		}
		out[i].Rank = i + 1
	}
	return out
}
