package tasks_test

import (
	"encoding/json"
	"testing"

	"tolerance/fixtures/tasks"
)

func TestLoad_CityDayPlanner(t *testing.T) {
	b, err := tasks.Load("city-day-planner")
	if err != nil {
		t.Fatal(err)
	}
	if b.Suite != "city-day-planner" || b.SuiteVersion != "1" {
		t.Fatalf("suite identity: %q@%q", b.Suite, b.SuiteVersion)
	}
	if len(b.Checks) != 8 {
		t.Fatalf("want 8 checks, got %d", len(b.Checks))
	}
	sum := 0
	for _, c := range b.Checks {
		sum += c.Weight
	}
	if sum != 100 {
		t.Fatalf("weights sum to %d", sum)
	}
	var task map[string]any
	if err := json.Unmarshal(b.Task, &task); err != nil {
		t.Fatal(err)
	}
	if reqs, _ := task["requirements"].([]any); len(reqs) != 7 {
		t.Fatalf("want 7 requirements R1-R7, got %d", len(reqs))
	}
	var places struct {
		Places []any `json:"places"`
	}
	if err := json.Unmarshal(b.Places, &places); err != nil || len(places.Places) != 24 {
		t.Fatalf("places: %v (%d)", err, len(places.Places))
	}
}

func TestLoad_UnknownSlug(t *testing.T) {
	if _, err := tasks.Load("nope"); err == nil {
		t.Fatal("unknown slug must fail")
	}
	if _, err := tasks.Load("../tasks"); err == nil {
		t.Fatal("path traversal in slug must fail")
	}
}

func TestValidate_RejectsBrokenBundles(t *testing.T) {
	good := tasks.CheckDef{ID: "a", Title: "t", Requirement: "R1", Weight: 100, Required: true}
	cases := map[string][]tasks.CheckDef{
		"weights not 100": {{ID: "a", Title: "t", Requirement: "R1", Weight: 50}},
		"duplicate id":    {{ID: "a", Title: "t", Requirement: "R1", Weight: 50}, {ID: "a", Title: "t", Requirement: "R1", Weight: 50}},
		"empty":           {},
	}
	for name, checks := range cases {
		if err := tasks.ValidateChecks(checks); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if err := tasks.ValidateChecks([]tasks.CheckDef{good}); err != nil {
		t.Errorf("valid checks rejected: %v", err)
	}
}
