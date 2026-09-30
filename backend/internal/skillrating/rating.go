// Package skillrating is the one place the platform's skill rating is
// defined (not to be confused with internal/games/rating, the tanks ladder's
// TrueSkill-style μ/σ).
// It is a pure function of run scores so it can be tested on a table and
// explained to an owner in two sentences: your rating is the average of
// your runs on this agent version, mapped onto 1000–2400; the ± shrinks
// with every run.
package skillrating

import "math"

const (
	Floor     = 1000
	Span      = 1400
	MaxUncert = 350
	MinUncert = 60

	TierVerified = 1500
	TierStrong   = 1800
	TierElite    = 2100
)

type State struct {
	Rating      int
	Uncertainty int
	Runs        int  // runs on the current version
	SumTargets  int  // sum of Target over those runs
	Prior       *int // rating carried from the previous version, nil on the first
}

// Target maps a run score in [0,1] onto the rating scale.
func Target(score float64) int {
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return Floor + int(math.Round(Span*score))
}

func Uncertainty(runs int) int {
	if runs <= 0 {
		return MaxUncert
	}
	u := int(math.Round(MaxUncert / math.Sqrt(float64(runs))))
	if u < MinUncert {
		return MinUncert
	}
	return u
}

// Apply folds one scored run into the state. The first run on a new
// version is averaged with the prior version's rating so a model change
// neither inherits the old number outright nor throws it away.
func Apply(s State, score float64) State {
	t := Target(score)
	s.Runs++
	s.SumTargets += t
	s.Rating = int(math.Round(float64(s.SumTargets) / float64(s.Runs)))
	if s.Runs == 1 && s.Prior != nil {
		s.Rating = int(math.Round(float64(*s.Prior+t) / 2))
	}
	s.Uncertainty = Uncertainty(s.Runs)
	return s
}

// Remove takes one scored run back out of the state — the exact inverse of
// Apply, for an administrator voiding a run. This is the only operation on the
// platform that changes a rating after the fact, which is why it is a named,
// tested function rather than an ad-hoc UPDATE. Removing the last run on a
// version leaves the previous version's rating standing with uncertainty back at
// its maximum, the same state NewVersion produces.
func Remove(s State, target int) State {
	if s.Runs <= 0 {
		return s
	}
	s.Runs--
	s.SumTargets -= target
	if s.Runs == 0 {
		s.SumTargets = 0
		s.Uncertainty = MaxUncert
		if s.Prior != nil {
			s.Rating = *s.Prior
			return s
		}
		// No prior and no runs left: the agent is simply unrated on this skill
		// again, and Rating goes back to zero rather than keeping a number no run
		// supports. A caller holding a stored rating should drop the row — see
		// Unrated.
		s.Rating = 0
		return s
	}
	s.Rating = int(math.Round(float64(s.SumTargets) / float64(s.Runs)))
	if s.Runs == 1 && s.Prior != nil {
		s.Rating = int(math.Round(float64(*s.Prior+s.SumTargets) / 2))
	}
	s.Uncertainty = Uncertainty(s.Runs)
	return s
}

// NewVersion is what happens to a rating when the agent's version changes.
func NewVersion(s State) State {
	prior := s.Rating
	return State{Rating: s.Rating, Uncertainty: MaxUncert, Prior: &prior}
}

// Unrated reports that a state carries no evidence at all: no runs on the
// current version and no rating inherited from an earlier one. A stored rating
// in this state should be deleted, not displayed — there is no number to show.
func Unrated(s State) bool { return s.Runs == 0 && s.Prior == nil }

func Access(rating, uncertainty int) int { return rating - uncertainty }

func Tier(access int) string {
	switch {
	case access >= TierElite:
		return "elite"
	case access >= TierStrong:
		return "strong"
	case access >= TierVerified:
		return "verified"
	}
	return "none"
}

func Verified(rating, uncertainty int) bool { return Access(rating, uncertainty) >= TierVerified }

// SkillRating is one skill's rating as the API shows it. VersionID and
// VersionNumber name the agent version the rating was earned on (the
// version of the latest scored run); OnCurrentVersion is false until a run
// on the agent's current version is scored.
type SkillRating struct {
	SkillSlug        string `json:"skill_slug"`
	Rating           int    `json:"rating"`
	Uncertainty      int    `json:"uncertainty"`
	Access           int    `json:"access"`
	Tier             string `json:"tier"`
	Verified         bool   `json:"verified"`
	Runs             int    `json:"runs"`
	VersionID        string `json:"version_id"`
	VersionNumber    int    `json:"version_number"`
	OnCurrentVersion bool   `json:"on_current_version"`
	PriorRating      *int   `json:"prior_rating"`
}
