package house

import (
	"container/heap"
	"math"

	"tolerance/internal/games/tanks"
)

// navField is a cost-to-go grid toward one goal point: the length of the
// shortest wall-avoiding route from each cell centre, with a soft penalty
// for hugging walls and field edges. The planner reads it at the end of
// every candidate trajectory, so "go around that wall" falls out of the
// same scoring as everything else instead of needing a steering heuristic.
type navField struct {
	w, h int
	d    []float64
}

const navBig = 1e3

type navItem struct {
	idx  int
	cost float64
}

type navHeap []navItem

func (h navHeap) Len() int            { return len(h) }
func (h navHeap) Less(i, j int) bool  { return h[i].cost < h[j].cost }
func (h navHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *navHeap) Push(x interface{}) { *h = append(*h, x.(navItem)) }
func (h *navHeap) Pop() interface{} {
	old := *h
	n := len(old)
	it := old[n-1]
	*h = old[:n-1]
	return it
}

// navBlockedGrid precomputes the per-cell step cost (inf for cells a tank
// cannot occupy) once per map.
func navBlockedGrid(rules tanks.Rules, walls []tanks.Rect) (int, int, []float64) {
	w, h := int(math.Ceil(rules.Width)), int(math.Ceil(rules.Height))
	cost := make([]float64, w*h)
	r := rules.TankRadius
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			cx, cy := float64(i)+0.5, float64(j)+0.5
			c := 1.0
			dmin := math.Inf(1)
			for _, wl := range walls {
				if d := distToRect(cx, cy, wl); d < dmin {
					dmin = d
				}
			}
			if cx < r-0.05 || cx > rules.Width-r+0.05 || cy < r-0.05 || cy > rules.Height-r+0.05 {
				// Centres within one radius of the edge are unreachable;
				// keep the cell passable at a steep cost so a tank that
				// sits against the edge still has a gradient to follow.
				c += 6
			}
			if dmin < r-0.05 {
				c += 6
			} else if dmin < r+1.0 {
				c += 2 * (r + 1.0 - dmin)
			}
			edge := math.Min(math.Min(cx, rules.Width-cx), math.Min(cy, rules.Height-cy))
			if edge < r+1.0 {
				c += 1.5 * (r + 1.0 - edge)
			}
			cost[j*w+i] = c
		}
	}
	return w, h, cost
}

// buildNav runs Dijkstra from the goal over the precomputed cell costs.
func buildNav(w, h int, cellCost []float64, gx, gy float64) *navField {
	f := &navField{w: w, h: h, d: make([]float64, w*h)}
	for i := range f.d {
		f.d[i] = math.Inf(1)
	}
	gi := clampInt(int(gx), 0, w-1)
	gj := clampInt(int(gy), 0, h-1)
	start := gj*w + gi
	f.d[start] = 0
	hp := &navHeap{{idx: start, cost: 0}}
	for hp.Len() > 0 {
		it := heap.Pop(hp).(navItem)
		if it.cost > f.d[it.idx] {
			continue
		}
		ci, cj := it.idx%w, it.idx/w
		for dj := -1; dj <= 1; dj++ {
			for di := -1; di <= 1; di++ {
				if di == 0 && dj == 0 {
					continue
				}
				ni, nj := ci+di, cj+dj
				if ni < 0 || nj < 0 || ni >= w || nj >= h {
					continue
				}
				step := 1.0
				if di != 0 && dj != 0 {
					step = 1.4142
					// no cutting corners between two blocked-ish cells
					if cellCost[cj*w+ni] > 5 || cellCost[nj*w+ci] > 5 {
						continue
					}
				}
				nidx := nj*w + ni
				nc := it.cost + step*cellCost[nidx]
				if nc < f.d[nidx] {
					f.d[nidx] = nc
					heap.Push(hp, navItem{idx: nidx, cost: nc})
				}
			}
		}
	}
	return f
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// at samples the field at a world position (bilinear between cell centres).
func (f *navField) at(x, y float64) float64 {
	fx, fy := x-0.5, y-0.5
	i0, j0 := int(math.Floor(fx)), int(math.Floor(fy))
	tx, ty := fx-float64(i0), fy-float64(j0)
	get := func(i, j int) float64 {
		i = clampInt(i, 0, f.w-1)
		j = clampInt(j, 0, f.h-1)
		v := f.d[j*f.w+i]
		if math.IsInf(v, 1) {
			return navBig
		}
		return v
	}
	a := get(i0, j0)*(1-tx) + get(i0+1, j0)*tx
	b := get(i0, j0+1)*(1-tx) + get(i0+1, j0+1)*tx
	return a*(1-ty) + b*ty
}
