package cityplanner_test

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"testing"

	"tolerance/internal/tasks/cityplanner"
)

const taskDir = "../../../fixtures/tasks/city-day-planner/"

type place struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	VisitMinutes int     `json:"visit_minutes"`
	Opens        string  `json:"opens"`
	Closes       string  `json:"closes"`
	CostEUR      int     `json:"cost_eur"`
}

type dataset struct {
	Start  place   `json:"start"`
	Places []place `json:"places"`
}

func loadDataset(t *testing.T) dataset {
	t.Helper()
	raw, err := os.ReadFile(taskDir + "places.json")
	if err != nil {
		t.Fatal(err)
	}
	var d dataset
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

var hhmm = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func minutes(s string) int {
	var h, m int
	h = int(s[0]-'0')*10 + int(s[1]-'0')
	m = int(s[3]-'0')*10 + int(s[4]-'0')
	return h*60 + m
}

func pt(p place) cityplanner.Point { return cityplanner.Point{Lat: p.Lat, Lng: p.Lng} }

// feasible lists places that can be visited alone from the start: the
// visit must fit inside opening hours and inside the time window, and the
// place must be affordable. Waiting for opening is allowed.
func feasible(d dataset, startTime string, windowMin, budget int) []string {
	var out []string
	for _, p := range d.Places {
		arrive := minutes(startTime) + cityplanner.TravelMinutes(pt(d.Start), pt(p))
		if o := minutes(p.Opens); arrive < o {
			arrive = o
		}
		limit := minutes(p.Closes)
		if end := minutes(startTime) + windowMin; end < limit {
			limit = end
		}
		if p.CostEUR <= budget && arrive+p.VisitMinutes <= limit {
			out = append(out, p.ID)
		}
	}
	return out
}

// greedy builds a plan by always taking the earliest-arrival feasible place.
func greedy(d dataset, startTime string, windowMin, budget int) []string {
	t, cur, left := minutes(startTime), d.Start, budget
	end := minutes(startTime) + windowMin
	used := map[string]bool{}
	var plan []string
	for {
		var best *place
		bestArrive := 0
		for i := range d.Places {
			p := d.Places[i]
			if used[p.ID] || p.CostEUR > left {
				continue
			}
			arrive := t + cityplanner.TravelMinutes(pt(cur), pt(p))
			if o := minutes(p.Opens); arrive < o {
				arrive = o
			}
			limit := minutes(p.Closes)
			if end < limit {
				limit = end
			}
			if arrive+p.VisitMinutes > limit {
				continue
			}
			if best == nil || arrive < bestArrive {
				best, bestArrive = &p, arrive
			}
		}
		if best == nil {
			return plan
		}
		plan = append(plan, best.ID)
		used[best.ID] = true
		t, cur, left = bestArrive+best.VisitMinutes, *best, left-best.CostEUR
	}
}

func TestDataset_Shape(t *testing.T) {
	d := loadDataset(t)
	if len(d.Places) != 24 {
		t.Fatalf("want 24 places, got %d", len(d.Places))
	}
	idRe := regexp.MustCompile(`^p\d{2}$`)
	cats := map[string]bool{"sight": true, "museum": true, "viewpoint": true, "food": true, "park": true, "shop": true}
	seen := map[string]bool{}
	free, food := 0, 0
	for _, p := range d.Places {
		if !idRe.MatchString(p.ID) || seen[p.ID] {
			t.Errorf("bad or duplicate id %q", p.ID)
		}
		seen[p.ID] = true
		if !cats[p.Category] {
			t.Errorf("%s: unknown category %q", p.ID, p.Category)
		}
		if !hhmm.MatchString(p.Opens) || !hhmm.MatchString(p.Closes) || minutes(p.Opens) >= minutes(p.Closes) {
			t.Errorf("%s: bad hours %s-%s", p.ID, p.Opens, p.Closes)
		}
		if p.VisitMinutes < 30 {
			t.Errorf("%s: visit_minutes %d < 30", p.ID, p.VisitMinutes)
		}
		if p.CostEUR < 0 {
			t.Errorf("%s: negative cost", p.ID)
		}
		if p.Lat < 44.40 || p.Lat > 44.43 || p.Lng < 8.91 || p.Lng > 8.95 {
			t.Errorf("%s: coordinates outside the city box", p.ID)
		}
		if p.CostEUR == 0 {
			free++
		}
		if p.Category == "food" {
			food++
		}
	}
	if free < 8 {
		t.Errorf("want at least 8 free places, got %d", free)
	}
	if food < 4 {
		t.Errorf("want at least 4 food places, got %d", food)
	}
}

func TestDataset_InfeasibleCases(t *testing.T) {
	d := loadDataset(t)
	if got := feasible(d, "09:00", 30, 1000); len(got) != 0 {
		t.Errorf("09:00/30min/1000: expected no feasible place, got %v", got)
	}
	if got := feasible(d, "23:30", 120, 0); len(got) != 0 {
		t.Errorf("23:30/120min/0: expected no feasible place, got %v", got)
	}
}

func TestDataset_FeasibleCases(t *testing.T) {
	d := loadDataset(t)
	if plan := greedy(d, "09:00", 480, 40); len(plan) < 3 {
		t.Errorf("09:00/480min/40: want at least 3 stops, got %v", plan)
	}
	plan := greedy(d, "09:00", 240, 0)
	if len(plan) < 2 {
		t.Fatalf("09:00/240min/0: want at least 2 stops, got %v", plan)
	}
	byID := map[string]place{}
	for _, p := range d.Places {
		byID[p.ID] = p
	}
	for _, id := range plan {
		if byID[id].CostEUR != 0 {
			t.Errorf("zero budget plan contains paid place %s", id)
		}
	}
}

func TestTravelMinutes_Properties(t *testing.T) {
	d := loadDataset(t)
	for _, p := range d.Places {
		if got := cityplanner.TravelMinutes(pt(p), pt(p)); got != 0 {
			t.Errorf("%s to itself: %d", p.ID, got)
		}
	}
	all := append([]place{d.Start}, d.Places...)
	for _, a := range all {
		for _, b := range all {
			if cityplanner.TravelMinutes(pt(a), pt(b)) != cityplanner.TravelMinutes(pt(b), pt(a)) {
				t.Errorf("not symmetric: %s %s", a.ID, b.ID)
			}
		}
	}
	byID := map[string]place{}
	for _, p := range d.Places {
		byID[p.ID] = p
	}
	if m := cityplanner.TravelMinutes(pt(d.Start), pt(byID["p08"])); m < 1 || m > 3 {
		t.Errorf("start to p08: want 1..3, got %d", m)
	}
	if m := cityplanner.TravelMinutes(pt(d.Start), pt(byID["p09"])); m < 18 || m > 24 {
		t.Errorf("start to p09: want 18..24, got %d", m)
	}
}

type vector struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Minutes int    `json:"minutes"`
}

// vectors is the set pinned for the TypeScript checker: start to every
// place, plus the first 40 place pairs in lexicographic (from, to) order.
func vectors(d dataset) []vector {
	pts := map[string]cityplanner.Point{d.Start.ID: pt(d.Start)}
	ids := make([]string, 0, len(d.Places))
	for _, p := range d.Places {
		pts[p.ID] = pt(p)
		ids = append(ids, p.ID)
	}
	sort.Strings(ids)
	var out []vector
	for _, id := range ids {
		out = append(out, vector{"start", id, cityplanner.TravelMinutes(pts["start"], pts[id])})
	}
	pairs := 0
	for i := 0; i < len(ids) && pairs < 40; i++ {
		for j := i + 1; j < len(ids) && pairs < 40; j++ {
			out = append(out, vector{ids[i], ids[j], cityplanner.TravelMinutes(pts[ids[i]], pts[ids[j]])})
			pairs++
		}
	}
	return out
}

func TestTravelVectors_Golden(t *testing.T) {
	want, err := json.MarshalIndent(vectors(loadDataset(t)), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	path := taskDir + "travel-vectors.json"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (generate with UPDATE_GOLDEN=1)", err)
	}
	if string(got) != string(want) {
		t.Fatal("travel-vectors.json differs from the formula; the checker copy of the rule must change together with it")
	}
}
