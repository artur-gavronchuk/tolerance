package fairplay

import (
	"hash/fnv"
	"regexp"
	"slices"
	"strings"
)

const (
	shingleSize   = 5    // tokens per shingle
	sketchSize    = 1000 // bottom-k: the k smallest shingle hashes stand in for the whole set
	maxTokens     = 400_000
	minUnique     = 20  // after discounting the common fix, fewer shingles than this prove nothing
	dupThreshold  = 0.9 // Jaccard similarity that raises a near_duplicate flag
	commonUsersAt = 3   // the "everyone makes this fix" discount needs at least this many other users
)

var tokenRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*|\d+|\S`)

// normalize reduces a solution to the lines that carry the change: only the added and removed lines of a diff
// (without the +/- marker); whitespace is collapsed so reformatting changes nothing.
func normalize(kind, content string) string {
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		{
			if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || line == "" || (line[0] != '+' && line[0] != '-') {
				continue
			}
			line = line[1:]
		}
		if line = strings.TrimSpace(line); line != "" {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// sketchOf returns the sorted bottom-k hashes of the token 5-shingles of a solution.
func sketchOf(kind, content string) []int64 {
	toks := tokenRe.FindAllString(normalize(kind, content), maxTokens)
	if len(toks) < shingleSize {
		return nil
	}
	set := make(map[int64]struct{}, len(toks))
	h := fnv.New64a()
	for i := 0; i+shingleSize <= len(toks); i++ {
		h.Reset()
		for _, t := range toks[i : i+shingleSize] {
			_, _ = h.Write([]byte(t))
			_, _ = h.Write([]byte{0})
		}
		set[int64(h.Sum64()>>1)] = struct{}{}
	}
	out := make([]int64, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	slices.Sort(out)
	if len(out) > sketchSize {
		out = out[:sketchSize]
	}
	return out
}

// commonShingles are the shingles present in more than half of the given sketches: the minimal fix almost
// everyone makes. Only meaningful with enough sketches, otherwise nil.
func commonShingles(sketches [][]int64) map[int64]bool {
	if len(sketches) < commonUsersAt {
		return nil
	}
	count := map[int64]int{}
	for _, sk := range sketches {
		for _, v := range sk {
			count[v]++
		}
	}
	common := map[int64]bool{}
	for v, c := range count {
		if c*2 > len(sketches) {
			common[v] = true
		}
	}
	return common
}

// jaccard of two sorted sketches after dropping the common shingles; ok is false when too little is left to compare.
func jaccard(a, b []int64, common map[int64]bool) (j float64, ok bool) {
	set := map[int64]bool{}
	for _, v := range a {
		if !common[v] {
			set[v] = true
		}
	}
	var na, inter int
	na = len(set)
	nb := 0
	for _, v := range b {
		if common[v] {
			continue
		}
		nb++
		if set[v] {
			inter++
		}
	}
	if na < minUnique || nb < minUnique {
		return 0, false
	}
	return float64(inter) / float64(na+nb-inter), true
}
