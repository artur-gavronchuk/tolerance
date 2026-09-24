package tanks

// Point is a 2D coordinate.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Rect is an axis-aligned rectangle; X,Y is its bottom-left corner.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Zone is the shrinking circle that forces the match to converge.
type Zone struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	R float64 `json:"r"`
}

// Map is a match map: walls plus the fixed spawn and bonus positions. Every
// map is symmetric about the field centre: spawn i and spawn i+2 are
// opposite each other, and every wall has a mirror wall in the same map.
type Map struct {
	Name    string
	Walls   []Rect
	Spawns  [4]Point // 0<->2 and 1<->3 are opposite, across the centre
	Bonuses [4]Point
}

// Maps returns the built-in map catalog, in a fixed order.
func Maps() []Map {
	return []Map{arenaMap(), crossroadsMap(), bunkersMap()}
}

// MapByName looks up a built-in map by name.
func MapByName(name string) (Map, bool) {
	for _, m := range Maps() {
		if m.Name == name {
			return m, true
		}
	}
	return Map{}, false
}

// PickMap deterministically selects a built-in map from a seed.
func PickMap(seed int64) Map {
	maps := Maps()
	n := int64(len(maps))
	idx := seed % n
	if idx < 0 {
		idx += n
	}
	return maps[idx]
}

// corners are the spawn points shared by every built-in map: four corners,
// inset from the field edge, diagonally opposite across the centre.
var corners = [4]Point{
	{X: 5, Y: 5},
	{X: 5, Y: 35},
	{X: 55, Y: 35},
	{X: 55, Y: 5},
}

// arenaMap is open ground with four square columns, one per quadrant.
func arenaMap() Map {
	return Map{
		Name: "arena",
		Walls: []Rect{
			{X: 15, Y: 12, W: 3, H: 3},
			{X: 15, Y: 25, W: 3, H: 3},
			{X: 42, Y: 25, W: 3, H: 3},
			{X: 42, Y: 12, W: 3, H: 3},
		},
		Spawns: corners,
		Bonuses: [4]Point{
			{X: 26, Y: 20},
			{X: 30, Y: 24},
			{X: 34, Y: 20},
			{X: 30, Y: 16},
		},
	}
}

// crossroadsMap is a cross of four wall arms with a passage through the
// centre, so tanks can still cut diagonally across the middle.
func crossroadsMap() Map {
	return Map{
		Name: "crossroads",
		Walls: []Rect{
			{X: 28, Y: 26, W: 4, H: 12}, // north arm
			{X: 28, Y: 2, W: 4, H: 12},  // south arm
			{X: 36, Y: 18, W: 12, H: 4}, // east arm
			{X: 12, Y: 18, W: 12, H: 4}, // west arm
		},
		Spawns: corners,
		Bonuses: [4]Point{
			{X: 20, Y: 28},
			{X: 40, Y: 28},
			{X: 40, Y: 12},
			{X: 20, Y: 12},
		},
	}
}

// bunkersMap has an L-shaped ("Г") shelter near each spawn plus a central
// block.
func bunkersMap() Map {
	return Map{
		Name: "bunkers",
		Walls: []Rect{
			{X: 8, Y: 5, W: 2, H: 8},   // near spawn 0
			{X: 8, Y: 5, W: 8, H: 2},   // near spawn 0
			{X: 50, Y: 27, W: 2, H: 8}, // near spawn 2
			{X: 44, Y: 33, W: 8, H: 2}, // near spawn 2
			{X: 8, Y: 27, W: 2, H: 8},  // near spawn 1
			{X: 8, Y: 33, W: 8, H: 2},  // near spawn 1
			{X: 50, Y: 5, W: 2, H: 8},  // near spawn 3
			{X: 44, Y: 5, W: 8, H: 2},  // near spawn 3
			{X: 27, Y: 17, W: 6, H: 6}, // centre block
		},
		Spawns: corners,
		Bonuses: [4]Point{
			{X: 18, Y: 20},
			{X: 30, Y: 30},
			{X: 42, Y: 20},
			{X: 30, Y: 10},
		},
	}
}
