package skills

import "math/rand"

// TasksPerRun is how many tasks one qualification run consists of. A skill
// with fewer active tasks cannot be qualified.
const TasksPerRun = 3

// Pick chooses n distinct task slugs from pool, preferring ones not in
// recent. When fewer than n unseen tasks exist, the rest come from the
// whole pool; when the pool itself is smaller than n, it is returned whole.
func Pick(pool, recent []string, n int, rnd *rand.Rand) []string {
	if len(pool) <= n {
		out := append([]string(nil), pool...)
		rnd.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		return out
	}
	seen := map[string]bool{}
	for _, r := range recent {
		seen[r] = true
	}
	var unseen, rest []string
	for _, p := range pool {
		if seen[p] {
			rest = append(rest, p)
		} else {
			unseen = append(unseen, p)
		}
	}
	rnd.Shuffle(len(unseen), func(i, j int) { unseen[i], unseen[j] = unseen[j], unseen[i] })
	rnd.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	out := append(unseen, rest...)
	return out[:n]
}
