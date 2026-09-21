// Package cityplanner holds the published rules of the city-day-planner
// task. The checker (TypeScript) implements the same formula; both are
// pinned by fixtures/tasks/city-day-planner/travel-vectors.json.
package cityplanner

import "math"

type Point struct{ Lat, Lng float64 }

const (
	earthRadiusKm = 6371.0088
	detourFactor  = 1.3
	walkKmPerHour = 4.8
)

// TravelMinutes is the published walking-time rule: haversine distance,
// times a 1.3 detour factor, at 4.8 km/h, rounded up to whole minutes.
// The same point is 0 minutes away.
func TravelMinutes(a, b Point) int {
	if a == b {
		return 0
	}
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat, dLng := rad(b.Lat-a.Lat), rad(b.Lng-a.Lng)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(a.Lat))*math.Cos(rad(b.Lat))*math.Sin(dLng/2)*math.Sin(dLng/2)
	km := 2 * earthRadiusKm * math.Asin(math.Sqrt(h))
	return int(math.Ceil(km * detourFactor / walkKmPerHour * 60))
}
