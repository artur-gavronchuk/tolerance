package tanks

import (
	"math"
	"testing"
)

const symEps = 1e-9

func reflectRect(width, height float64, r Rect) Rect {
	return Rect{X: width - r.X - r.W, Y: height - r.Y - r.H, W: r.W, H: r.H}
}

func rectsEqual(a, b Rect) bool {
	return math.Abs(a.X-b.X) < symEps && math.Abs(a.Y-b.Y) < symEps &&
		math.Abs(a.W-b.W) < symEps && math.Abs(a.H-b.H) < symEps
}

func distToRect(p Point, r Rect) float64 {
	inside := p.X >= r.X && p.X <= r.X+r.W && p.Y >= r.Y && p.Y <= r.Y+r.H
	if inside {
		return 0
	}
	cx := math.Max(r.X, math.Min(p.X, r.X+r.W))
	cy := math.Max(r.Y, math.Min(p.Y, r.Y+r.H))
	return math.Hypot(p.X-cx, p.Y-cy)
}

func TestMapsSymmetric(t *testing.T) {
	for _, m := range Maps() {
		t.Run(m.Name, func(t *testing.T) {
			width, height := DefaultRules().Width, DefaultRules().Height

			for _, w := range m.Walls {
				mirror := reflectRect(width, height, w)
				found := false
				for _, w2 := range m.Walls {
					if rectsEqual(mirror, w2) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("wall %+v has no mirror wall %+v in map %s", w, mirror, m.Name)
				}
			}

			for i := 0; i < 2; i++ {
				a, b := m.Spawns[i], m.Spawns[i+2]
				wantB := Point{X: width - a.X, Y: height - a.Y}
				if math.Abs(b.X-wantB.X) > symEps || math.Abs(b.Y-wantB.Y) > symEps {
					t.Errorf("spawn %d = %+v, spawn %d = %+v, not centrally symmetric", i, a, i+2, b)
				}
			}

			checkPoint := func(label string, p Point) {
				for _, w := range m.Walls {
					if p.X >= w.X && p.X <= w.X+w.W && p.Y >= w.Y && p.Y <= w.Y+w.H {
						t.Errorf("%s %+v is inside wall %+v", label, p, w)
					}
					if d := distToRect(p, w); d < 1.5-symEps {
						t.Errorf("%s %+v is %.3f from wall %+v, want >= 1.5", label, p, d, w)
					}
				}
			}
			for i, p := range m.Spawns {
				checkPoint("spawn", p)
				_ = i
			}
			for i, p := range m.Bonuses {
				checkPoint("bonus", p)
				_ = i
			}
		})
	}
}

func TestPickMap(t *testing.T) {
	if got := PickMap(0).Name; got != "arena" {
		t.Errorf("PickMap(0).Name = %q, want arena", got)
	}
	if got := PickMap(4).Name; got != "crossroads" {
		t.Errorf("PickMap(4).Name = %q, want crossroads", got)
	}
}
