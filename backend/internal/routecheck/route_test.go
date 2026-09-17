package routecheck_test

import (
	"encoding/json"
	"testing"

	"tolerance/fixtures/city"
	"tolerance/internal/routecheck"
)

func exampleRoute(t *testing.T) routecheck.Route {
	t.Helper()
	var r routecheck.Route
	if err := json.Unmarshal(city.Example, &r); err != nil {
		t.Fatalf("unmarshal example route: %v", err)
	}
	return r
}

func TestValidateRoute_AcceptsTheReferenceExample(t *testing.T) {
	d := routecheck.CityDataset()
	r := exampleRoute(t)
	result := routecheck.ValidateRoute(r, d)
	if result.Status != "pass" {
		t.Fatalf("expected reference route to pass, got %s: %v", result.Status, result.Observations)
	}
}

func TestValidateRoute_AcceptsAnAlternativeValidOrder(t *testing.T) {
	// The validator must not compare against a single reference answer: a
	// different feasible order of the same places must also pass.
	d := routecheck.CityDataset()
	r := routecheck.Route{
		DatasetVersion: d.Version, Date: d.Date, Start: 600, TimeLimit: 180, WalkLimit: 60,
		Interests: []string{"nature", "food", "culture"},
		Stops: []routecheck.Stop{
			{PlaceID: "park", Arrival: 615, Departure: 645},
			{PlaceID: "cafe", Arrival: 653, Departure: 683},
			{PlaceID: "museum", Arrival: 709, Departure: 754}, // no direct cafe→museum edge; shortest path is via park (8+18)
		},
	}
	result := routecheck.ValidateRoute(r, d)
	if result.Status != "pass" {
		t.Fatalf("expected alternative valid order to pass, got %s: %v", result.Status, result.Observations)
	}
}

func TestValidateRoute_RejectsMissingEdgeAsUnreachable(t *testing.T) {
	d := routecheck.Dataset{
		Version: "v1", Date: "2026-09-17", StartNode: "a",
		Places: []routecheck.Place{{ID: "a", Name: "A", Category: "x", Opens: 0, Closes: 1440, VisitMinutes: 0},
			{ID: "b", Name: "B", Category: "x", Opens: 0, Closes: 1440, VisitMinutes: 10}},
		Edges: []routecheck.Edge{}, // no edge a -> b at all
	}
	r := routecheck.Route{DatasetVersion: "v1", Date: "2026-09-17", Start: 0, TimeLimit: 120, WalkLimit: 60,
		Interests: []string{"x"}, Stops: []routecheck.Stop{{PlaceID: "b", Arrival: 0, Departure: 10}}}
	result := routecheck.ValidateRoute(r, d)
	if result.Status != "fail" {
		t.Fatalf("expected unreachable place to fail, got %s", result.Status)
	}
}

func TestValidateRoute_RejectsVisitOutsideOpeningHours(t *testing.T) {
	d := routecheck.CityDataset()
	r := exampleRoute(t)
	r.Stops[0].Departure = d.Places[0].Closes + 60 // push the first visit past closing
	result := routecheck.ValidateRoute(r, d)
	if result.Status != "fail" {
		t.Fatalf("expected visit past closing time to fail")
	}
}

func TestValidateRoute_RejectsExceedingTheWalkLimit(t *testing.T) {
	d := routecheck.CityDataset()
	r := exampleRoute(t)
	r.WalkLimit = 0
	result := routecheck.ValidateRoute(r, d)
	if result.Status != "fail" {
		t.Fatalf("expected zero walk limit against a route that walks to fail")
	}
}

func TestValidateRoute_RejectsUnknownInterestCategory(t *testing.T) {
	d := routecheck.CityDataset()
	r := exampleRoute(t)
	r.Interests = []string{"space-tourism"}
	result := routecheck.ValidateRoute(r, d)
	if result.Status != "fail" {
		t.Fatalf("expected unknown interest category to fail")
	}
}
