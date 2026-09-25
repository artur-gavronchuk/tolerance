// Package rating implements the Weng–Lin (Plackett–Luce) skill rating model
// (Weng & Lin, 2011), as used by OpenSkill, for free-for-all matches of 2–4
// bots. Pure functions, stdlib only.
package rating

import "math"

const (
	DefaultMu    = 25.0
	DefaultSigma = 25.0 / 3
	Beta         = DefaultSigma / 2
	Kappa        = 0.0001
	RefreshSigma = 5.0
)

// Rating is a single bot's skill estimate: mean (Mu) and uncertainty (Sigma).
type Rating struct{ Mu, Sigma float64 }

// Default returns the rating assigned to a bot that has never played.
func Default() Rating { return Rating{DefaultMu, DefaultSigma} }

// Display is the number shown to people: 1000 at the start, higher is better.
func Display(r Rating) int { return int(math.Round(1000 + 40*(r.Mu-3*r.Sigma))) }

// Refresh widens the uncertainty when a bot ships a new version.
func Refresh(r Rating) Rating { return Rating{r.Mu, math.Max(r.Sigma, RefreshSigma)} }

// Update applies one free-for-all result with the Plackett–Luce model of Weng & Lin (2011), as in
// OpenSkill. places[i] is player i's finishing place, 1 is best; equal places are a tie.
//
// Update does not mutate rs or places. len(places) must equal len(rs); a mismatch is a
// programmer error and Update panics (via an out-of-range index) rather than silently
// producing a wrong result.
func Update(rs []Rating, places []int) []Rating {
	n := len(rs)
	if len(places) != n {
		panic("rating.Update: len(places) != len(rs)")
	}
	c2 := 0.0
	for _, r := range rs {
		c2 += r.Sigma*r.Sigma + Beta*Beta
	}
	c := math.Sqrt(c2)
	sumQ := make([]float64, n) // Σ exp(mu_s/c) over s placed no better than q
	a := make([]float64, n)    // how many share q's place
	for q := 0; q < n; q++ {
		for s := 0; s < n; s++ {
			if places[s] >= places[q] {
				sumQ[q] += math.Exp(rs[s].Mu / c)
			}
			if places[s] == places[q] {
				a[q]++
			}
		}
	}
	out := make([]Rating, n)
	for i := 0; i < n; i++ {
		ei := math.Exp(rs[i].Mu / c)
		omega, delta := 0.0, 0.0
		for q := 0; q < n; q++ {
			if places[q] > places[i] {
				continue
			}
			quot := ei / sumQ[q]
			if q == i {
				omega += (1 - quot) / a[q]
			} else {
				omega -= quot / a[q]
			}
			delta += quot * (1 - quot) / a[q]
		}
		s2 := rs[i].Sigma * rs[i].Sigma
		omega *= s2 / c
		delta *= s2 / c2
		gamma := rs[i].Sigma / c
		delta *= gamma
		out[i] = Rating{Mu: rs[i].Mu + omega, Sigma: rs[i].Sigma * math.Sqrt(math.Max(1-delta, Kappa))}
	}
	return out
}
