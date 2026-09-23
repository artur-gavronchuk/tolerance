package checks_test

import (
	"testing"

	"tolerance/internal/checks"
	"tolerance/internal/competitions"
	"tolerance/internal/submissions"
)

func results(statuses ...string) []checks.Result {
	// Eight checks with the manifest weights 15,15,15,10,10,10,10,15.
	weights := []int{15, 15, 15, 10, 10, 10, 10, 15}
	out := make([]checks.Result, len(statuses))
	for i, s := range statuses {
		out[i] = checks.Result{Status: s, Weight: weights[i]}
	}
	return out
}

func repeat(n int, s string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func TestEffectiveStatus(t *testing.T) {
	if got := checks.EffectiveStatus(checks.Result{Status: "failed"}); got != "failed" {
		t.Fatalf("no override: %s", got)
	}
	if got := checks.EffectiveStatus(checks.Result{Status: "failed", OverrideStatus: "passed"}); got != "passed" {
		t.Fatalf("an admin override wins: %s", got)
	}
	if got := checks.EffectiveStatus(checks.Result{Status: "passed", OverrideStatus: "insufficient_data"}); got != "insufficient_data" {
		t.Fatalf("override to insufficient_data: %s", got)
	}
}

func TestFunctionalScore(t *testing.T) {
	cases := []struct {
		name string
		in   []checks.Result
		want int
	}{
		{"all passed", results(repeat(8, "passed")...), 100},
		{"mobile failed costs its weight of 15", results("passed", "passed", "passed", "passed", "passed", "passed", "passed", "failed"), 85},
		{"insufficient data earns nothing", results("passed", "passed", "passed", "insufficient_data", "insufficient_data", "passed", "passed", "passed"), 80},
		{"nothing passed", results(repeat(8, "failed")...), 0},
		{"only the three heavy checks passed", results("passed", "passed", "passed", "failed", "failed", "failed", "failed", "failed"), 45},
		{"no results at all", nil, 0},
	}
	for _, c := range cases {
		if got := checks.FunctionalScore(c.in); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
	overridden := results(repeat(8, "passed")...)
	overridden[7].Status, overridden[7].OverrideStatus = "failed", "passed"
	if got := checks.FunctionalScore(overridden); got != 100 {
		t.Errorf("an override failed→passed must count: %d", got)
	}
	overridden[0].OverrideStatus = "failed"
	if got := checks.FunctionalScore(overridden); got != 85 {
		t.Errorf("an override passed→failed must count: %d", got)
	}
}

func TestFunctionalScore_RoundsHalfUp(t *testing.T) {
	// 1 of 8 checks weighted 1 each is 12.5, which rounds to 13.
	in := make([]checks.Result, 8)
	for i := range in {
		in[i] = checks.Result{Status: "failed", Weight: 1}
	}
	in[0].Status = "passed"
	if got := checks.FunctionalScore(in); got != 13 {
		t.Fatalf("12.5 rounds up, got %d", got)
	}
}

var criteria = []competitions.Criterion{
	{Name: "Functionality", Weight: 60, Source: "checks"},
	{Name: "UX & polish", Weight: 20, Source: "llm"},
	{Name: "Code quality", Weight: 10, Source: "llm"},
	{Name: "Creativity", Weight: 10, Source: "llm"},
}

func rated(name string, score int) submissions.ScoreEntry {
	return submissions.ScoreEntry{Name: name, Score: &score, Status: "rated"}
}

func notRated(name string) submissions.ScoreEntry {
	return submissions.ScoreEntry{Name: name, Status: "not_rated"}
}

func TestTotal(t *testing.T) {
	cases := []struct {
		name   string
		scores []submissions.ScoreEntry
		want   int
		wantOK bool
	}{
		{"only functionality rated is its own score", []submissions.ScoreEntry{rated("Functionality", 80), notRated("UX & polish"), notRated("Code quality"), notRated("Creativity")}, 80, true},
		{"all rated is the weighted mean", []submissions.ScoreEntry{rated("Functionality", 80), rated("UX & polish", 70), rated("Code quality", 60), rated("Creativity", 50)}, 73, true},
		{"unrated criteria do not drag the total down", []submissions.ScoreEntry{rated("Functionality", 100), rated("UX & polish", 50), notRated("Code quality"), notRated("Creativity")}, 88, true},
		{"functionality not rated means no total", []submissions.ScoreEntry{notRated("Functionality"), rated("UX & polish", 90)}, 0, false},
		{"functionality missing means no total", []submissions.ScoreEntry{rated("UX & polish", 90)}, 0, false},
		{"nothing rated", []submissions.ScoreEntry{notRated("Functionality")}, 0, false},
		{"zero is a valid score", []submissions.ScoreEntry{rated("Functionality", 0), notRated("UX & polish")}, 0, true},
		{"perfect", []submissions.ScoreEntry{rated("Functionality", 100), rated("UX & polish", 100), rated("Code quality", 100), rated("Creativity", 100)}, 100, true},
	}
	for _, c := range cases {
		got, ok := checks.Total(criteria, c.scores)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("%s: got (%d, %v), want (%d, %v)", c.name, got, ok, c.want, c.wantOK)
		}
	}
}

func TestTotal_UnknownCriterionNamesAreIgnored(t *testing.T) {
	got, ok := checks.Total(criteria, []submissions.ScoreEntry{rated("Functionality", 60), rated("Vibes", 100)})
	if !ok || got != 60 {
		t.Fatalf("a score for a criterion the competition does not have must not count: (%d, %v)", got, ok)
	}
}

func TestPointsAwarded(t *testing.T) {
	cases := []struct {
		points, total int
		kind          string
		want          int
	}{
		{500, 100, "official", 500},
		{500, 73, "official", 365},
		{500, 0, "official", 0},
		{500, 91, "official", 455},
		{333, 50, "official", 167}, // 166.5 rounds up
		{500, 100, "practice", 0},
		{500, 73, "practice", 0},
	}
	for _, c := range cases {
		if got := checks.PointsAwarded(c.points, c.total, c.kind); got != c.want {
			t.Errorf("PointsAwarded(%d, %d, %s) = %d, want %d", c.points, c.total, c.kind, got, c.want)
		}
	}
}
