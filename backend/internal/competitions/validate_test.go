package competitions_test

import (
	"encoding/json"
	"testing"
	"time"

	"tolerance/internal/competitions"
)

func valid() competitions.Input {
	return competitions.Input{Slug: "weekend-planner", Title: "T", Summary: "S", Brief: "B", Category: "Full build", Difficulty: "Hard",
		Points: 500, Deadline: time.Now().Add(48 * time.Hour), MatchDurationSeconds: 900,
		Criteria: []competitions.Criterion{{Name: "Functionality", Weight: 60, Description: "d"}, {Name: "UX & polish", Weight: 40, Description: "d"}}}
}

func TestValidate(t *testing.T) {
	now := time.Now()
	if err := competitions.Validate(valid(), now); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	cases := map[string]func(*competitions.Input){
		"slug":          func(i *competitions.Input) { i.Slug = "Bad Slug" },
		"category":      func(i *competitions.Input) { i.Category = "Other" },
		"difficulty":    func(i *competitions.Input) { i.Difficulty = "Insane" },
		"points":        func(i *competitions.Input) { i.Points = 0 },
		"deadline past": func(i *competitions.Input) { i.Deadline = now.Add(-time.Hour) },
		"duration":      func(i *competitions.Input) { i.MatchDurationSeconds = 10 },
		"weights sum":   func(i *competitions.Input) { i.Criteria[0].Weight = 50 },
		"dup criterion": func(i *competitions.Input) { i.Criteria[1].Name = "Functionality" },
		"no criteria":   func(i *competitions.Input) { i.Criteria = nil },
		"11 criteria":   func(i *competitions.Input) { i.Criteria = make11() },
		"zero weight": func(i *competitions.Input) {
			i.Criteria = []competitions.Criterion{{Name: "A", Weight: 100, Description: "d"}, {Name: "B", Weight: 0, Description: "d"}}
		},
	}
	for name, mutate := range cases {
		in := valid()
		mutate(&in)
		if err := competitions.Validate(in, now); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func make11() []competitions.Criterion {
	out := make([]competitions.Criterion, 11)
	for i := range out {
		out[i] = competitions.Criterion{Name: string(rune('A' + i)), Weight: 9, Description: "d"}
	}
	out[10].Weight = 10
	return out
}

func withSuite() competitions.Input {
	in := valid()
	in.CheckSuite = "city-day-planner"
	in.Task = json.RawMessage(`{"version":"1"}`)
	in.Criteria[0].Source = "checks"
	in.Criteria[1].Source = "llm"
	return in
}

func TestValidate_CriterionSourceAndSuite(t *testing.T) {
	now := time.Now()
	in := withSuite()
	if err := competitions.Validate(in, now); err != nil {
		t.Fatalf("competition with a suite rejected: %v", err)
	}

	plain := valid()
	if err := competitions.Validate(plain, now); err != nil {
		t.Fatal(err)
	}
	if plain.Criteria[0].Source != "llm" || plain.Criteria[1].Source != "llm" {
		t.Fatalf("an empty source defaults to llm, got %q and %q", plain.Criteria[0].Source, plain.Criteria[1].Source)
	}

	cases := map[string]func(*competitions.Input){
		"unknown source":         func(i *competitions.Input) { i.Criteria[1].Source = "vibes" },
		"two checks criteria":    func(i *competitions.Input) { i.Criteria[1].Source = "checks" },
		"checks without suite":   func(i *competitions.Input) { i.CheckSuite = ""; i.Task = nil },
		"unknown suite":          func(i *competitions.Input) { i.CheckSuite = "no-such-suite" },
		"suite without task":     func(i *competitions.Input) { i.Task = nil },
		"task is not an object":  func(i *competitions.Input) { i.Task = json.RawMessage(`[1]`) },
		"suite without checks":   func(i *competitions.Input) { i.Criteria[0].Source = "llm" },
		"suite path in the name": func(i *competitions.Input) { i.CheckSuite = "../seed" },
	}
	for name, mutate := range cases {
		in := withSuite()
		mutate(&in)
		if err := competitions.Validate(in, now); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
