package checks

import (
	"tolerance/internal/competitions"
	"tolerance/internal/submissions"
)

// Check statuses, as in the published statuses of a check.
const (
	StatusPassed           = "passed"
	StatusFailed           = "failed"
	StatusInsufficientData = "insufficient_data"
	StatusInfraError       = "infra_error"
)

// Result is the part of a check result that scoring needs.
type Result struct {
	Status         string
	OverrideStatus string // empty when the admin did not override
	Weight         int
}

// EffectiveStatus is the admin's override when there is one, otherwise the
// checker's verdict.
func EffectiveStatus(r Result) string {
	if r.OverrideStatus != "" {
		return r.OverrideStatus
	}
	return r.Status
}

// FunctionalScore is the weighted share of passed checks, 0-100, rounded
// half up. Failed and insufficient-data checks earn nothing. All arithmetic
// is on integers so rounding never depends on floating point.
func FunctionalScore(results []Result) int {
	var passed, all int
	for _, r := range results {
		all += r.Weight
		if EffectiveStatus(r) == StatusPassed {
			passed += r.Weight
		}
	}
	if all == 0 {
		return 0
	}
	return (200*passed + all) / (2 * all)
}

// Total is the weighted mean over the criteria that were rated, rounded
// half up. Criteria that are not rated leave the mean untouched instead of
// counting as zero. ok is false when the criterion measured by checks is not
// rated (there is nothing to base a total on) or nothing at all was rated.
func Total(criteria []competitions.Criterion, scores []submissions.ScoreEntry) (total int, ok bool) {
	byName := map[string]submissions.ScoreEntry{}
	for _, s := range scores {
		byName[s.Name] = s
	}
	var sum, weights int
	for _, c := range criteria {
		s, has := byName[c.Name]
		isRated := has && s.Status == "rated" && s.Score != nil
		if !isRated {
			if c.Source == competitions.SourceChecks {
				return 0, false
			}
			continue
		}
		sum += c.Weight * *s.Score
		weights += c.Weight
	}
	if weights == 0 {
		return 0, false
	}
	return (2*sum + weights) / (2 * weights), true
}

// PointsAwarded converts a total into season points. Only official attempts
// earn points; a practice attempt never changes standings.
func PointsAwarded(competitionPoints, total int, attemptKind string) int {
	if attemptKind != "official" {
		return 0
	}
	return (2*competitionPoints*total + 100) / 200
}
