// Package routecheck implements the B-G2 route-feasibility scenario for the
// "Несколько часов в незнакомом городе" pilot season. It is pure and has no
// dependency on storage or HTTP; the evaluation module (slice 2) wires it up
// as one scenario inside a scenario bundle.
package routecheck

import (
	"encoding/json"
	"fmt"
	"math"

	"tolerance/fixtures/city"
)

type Place struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	Opens        int    `json:"opens"`
	Closes       int    `json:"closes"`
	VisitMinutes int    `json:"visit_minutes"`
}

type Edge struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Minutes int    `json:"minutes"`
}

type Dataset struct {
	Version   string  `json:"version"`
	Date      string  `json:"date"`
	Timezone  string  `json:"timezone"`
	StartNode string  `json:"start_node"`
	Places    []Place `json:"places"`
	Edges     []Edge  `json:"edges"`
}

type Stop struct {
	PlaceID   string `json:"place_id"`
	Arrival   int    `json:"arrival_minute"`
	Departure int    `json:"departure_minute"`
}

type Route struct {
	DatasetVersion string   `json:"dataset_version"`
	Date           string   `json:"date"`
	Start          int      `json:"start_minute"`
	TimeLimit      int      `json:"time_limit_minutes"`
	WalkLimit      int      `json:"walk_limit_minutes"`
	Interests      []string `json:"interests"`
	Stops          []Stop   `json:"stops"`
}

// Result mirrors the requirement × scenario check_result shape without
// depending on the evaluation module's storage types.
type Result struct {
	Status       string   `json:"status"` // pass | fail
	Observations []string `json:"observations"`
}

// CityDataset returns the public synthetic snapshot embedded in fixtures/city.
func CityDataset() Dataset {
	var d Dataset
	if err := json.Unmarshal(city.Snapshot, &d); err != nil {
		panic(err)
	}
	return d
}

// ValidateRoute evaluates feasibility (requirement B-G2), not optimality,
// UI or persistence. It independently recomputes route properties from the
// graph rather than comparing against a single reference answer.
func ValidateRoute(r Route, d Dataset) Result {
	c := Result{Status: "pass", Observations: []string{}}
	fail := func(message string) { c.Status = "fail"; c.Observations = append(c.Observations, message) }
	if r.DatasetVersion != d.Version || r.Date != d.Date {
		fail("Версия данных или дата не совпадает с контрактом.")
	}
	if r.Start < 0 || r.Start >= 1440 || r.TimeLimit < 120 || r.TimeLimit > 360 || r.WalkLimit < 0 || r.WalkLimit > 360 {
		fail("Входные ограничения неверны: старт 0–1439, длительность 120–360, ходьба 0–360 минут.")
		return c
	}
	if len(r.Stops) == 0 || len(r.Stops) > 20 {
		fail("Маршрут должен содержать от 1 до 20 посещений.")
		return c
	}
	places := map[string]Place{}
	categories := map[string]bool{}
	for _, p := range d.Places {
		places[p.ID] = p
		categories[p.Category] = true
	}
	if len(r.Interests) == 0 {
		fail("Не указаны интересы.")
	}
	for _, interest := range r.Interests {
		if !categories[interest] {
			fail("Неизвестная категория интересов: " + interest)
		}
	}
	previous := d.StartNode
	minute := r.Start
	walking := 0
	seen := map[string]bool{}
	for _, stop := range r.Stops {
		p, ok := places[stop.PlaceID]
		if !ok {
			fail("Неизвестное место: " + stop.PlaceID)
			continue
		}
		if seen[p.ID] {
			fail("Повторное посещение места: " + p.ID)
		}
		seen[p.ID] = true
		travel, reachable := shortestPath(d, previous, p.ID)
		if !reachable {
			fail(fmt.Sprintf("Нет пути %s → %s.", previous, p.ID))
			continue
		}
		walking += travel
		expected := minute + travel
		if stop.Arrival != expected {
			fail(fmt.Sprintf("%s: прибытие должно быть в %d минут, получено %d.", p.Name, expected, stop.Arrival))
		}
		if stop.Arrival < 0 || stop.Departure > 1440 || stop.Departure != stop.Arrival+p.VisitMinutes {
			fail(p.Name + ": неверная длительность посещения.")
		}
		if stop.Arrival < p.Opens || stop.Departure > p.Closes {
			fail(p.Name + ": посещение вне часов работы.")
		}
		minute = stop.Departure
		previous = p.ID
	}
	if minute-r.Start > r.TimeLimit {
		fail("Маршрут превышает доступное время.")
	}
	if walking > r.WalkLimit {
		fail("Превышен лимит ходьбы.")
	}
	if c.Status == "pass" {
		c.Observations = append(c.Observations, fmt.Sprintf("%d посещения; %d минут всего; %d минут ходьбы. Часы работы и переходы соблюдены.", len(r.Stops), minute-r.Start, walking))
	}
	return c
}

func shortestPath(d Dataset, from, to string) (int, bool) {
	distances := map[string]int{from: 0}
	visited := map[string]bool{}
	for {
		node := ""
		best := math.MaxInt
		for n, value := range distances {
			if !visited[n] && value < best {
				node = n
				best = value
			}
		}
		if node == "" {
			return 0, false
		}
		if node == to {
			return best, true
		}
		visited[node] = true
		for _, e := range d.Edges {
			if e.From != node {
				continue
			}
			next := best + e.Minutes
			old, ok := distances[e.To]
			if !ok || next < old {
				distances[e.To] = next
			}
		}
	}
}
