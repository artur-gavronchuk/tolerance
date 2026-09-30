package house

import (
	"math"

	"tolerance/internal/games/tanks"
)

func clamp1(v float64) float64 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}

// findTank returns the tank with the given id, if present.
func findTank(ts []tanks.TankView, id int) (tanks.TankView, bool) {
	for _, t := range ts {
		if t.ID == id {
			return t, true
		}
	}
	return tanks.TankView{}, false
}

// nearestEnemy returns the closest living tank other than me, or nil.
func nearestEnemy(me tanks.TankView, ts []tanks.TankView) *tanks.TankView {
	var best *tanks.TankView
	bestD := math.Inf(1)
	for i := range ts {
		t := ts[i]
		if t.ID == me.ID || !t.Alive {
			continue
		}
		if d := distance(me.X, me.Y, t.X, t.Y); d < bestD {
			bestD = d
			cp := t
			best = &cp
		}
	}
	return best
}

func angleTo(x, y, tx, ty float64) float64 {
	return math.Atan2(ty-y, tx-x)
}

// normalizeAngle brings a into (-pi, pi], the same convention the engine
// itself uses.
func normalizeAngle(a float64) float64 {
	a = math.Mod(a, 2*math.Pi)
	if a <= -math.Pi {
		a += 2 * math.Pi
	} else if a > math.Pi {
		a -= 2 * math.Pi
	}
	return a
}

// angleDiff is the signed shortest difference a - b, normalized to
// (-pi, pi].
func angleDiff(a, b float64) float64 {
	return normalizeAngle(a - b)
}

func distance(x, y, tx, ty float64) float64 {
	return math.Hypot(tx-x, ty-y)
}

// leadTarget returns the aim point that leads a target moving at
// (targetVX, targetVY) so a shell fired now at shellSpeed from
// (shooterX, shooterY) meets it, solving the intercept quadratic. It falls
// back to the target's current position when there is no positive-time
// solution (e.g. the target outruns the shell).
func leadTarget(shooterX, shooterY, targetX, targetY, shellSpeed, targetVX, targetVY float64) (float64, float64) {
	dx := targetX - shooterX
	dy := targetY - shooterY

	a := targetVX*targetVX + targetVY*targetVY - shellSpeed*shellSpeed
	b := 2 * (dx*targetVX + dy*targetVY)
	c := dx*dx + dy*dy

	var t float64
	found := false
	if math.Abs(a) < 1e-9 {
		if math.Abs(b) > 1e-9 {
			if cand := -c / b; cand > 0 {
				t, found = cand, true
			}
		}
	} else {
		disc := b*b - 4*a*c
		if disc >= 0 {
			sq := math.Sqrt(disc)
			t1 := (-b + sq) / (2 * a)
			t2 := (-b - sq) / (2 * a)
			for _, cand := range []float64{t1, t2} {
				if cand > 0 && (!found || cand < t) {
					t, found = cand, true
				}
			}
		}
	}

	if !found {
		return targetX, targetY
	}
	return targetX + targetVX*t, targetY + targetVY*t
}

// shellClosestApproach returns the smallest distance shell (at position
// px,py, velocity vx,vy units/s) comes to a static point mx,my within the
// next maxSeconds, assuming it keeps flying in a straight line.
func shellClosestApproach(px, py, vx, vy, mx, my, maxSeconds float64) float64 {
	dx, dy := px-mx, py-my
	speed2 := vx*vx + vy*vy
	if speed2 < 1e-9 {
		return math.Hypot(dx, dy)
	}
	t := -(dx*vx + dy*vy) / speed2
	t = math.Max(0, math.Min(maxSeconds, t))
	cx, cy := px+vx*t-mx, py+vy*t-my
	return math.Hypot(cx, cy)
}

// Wall avoidance. bunkers's per-spawn L-shaped walls trap any strategy that
// only ever turns straight toward its target — see qualify.go's comment for
// how badly. pathClear and steerAround give a strategy a cheap way to check
// a line against the map and route around an obstacle instead of shoving
// into it; the starter kits' tanks.py/tanks.js ship the same two functions.

const (
	// steerMargin pads a wall beyond the tank's own radius when checking
	// whether a path is clear, so a "clear" path still leaves some
	// clearance rather than grazing a corner.
	steerMargin = 0.4
	// steerLookahead is how far ahead a candidate heading is checked
	// before being accepted, in units.
	steerLookahead = 6.0
)

// steerOffsetsDeg are the headings (in degrees, off the direct heading)
// tried in order when the direct path is blocked, nearest first.
var steerOffsetsDeg = []float64{30, 60, 90, 120}

// segmentClearsRect reports whether the segment (x1,y1)-(x2,y2) stays clear
// of rect inflated by pad on every side, using the standard Liang-Barsky
// segment-vs-AABB clipping test: it walks the segment's parametric range
// [0,1] through each of the rect's four half-plane constraints, and the
// segment only enters the rect if a non-empty sub-range survives all four.
func segmentClearsRect(x1, y1, x2, y2 float64, r tanks.Rect, pad float64) bool {
	rx0, ry0 := r.X-pad, r.Y-pad
	rx1, ry1 := r.X+r.W+pad, r.Y+r.H+pad
	dx, dy := x2-x1, y2-y1
	t0, t1 := 0.0, 1.0
	edges := [4][2]float64{
		{-dx, x1 - rx0},
		{dx, rx1 - x1},
		{-dy, y1 - ry0},
		{dy, ry1 - y1},
	}
	for _, e := range edges {
		p, q := e[0], e[1]
		if math.Abs(p) < 1e-9 {
			if q < 0 {
				return true // parallel to this edge and outside it: never enters
			}
			continue
		}
		r := q / p
		if p < 0 {
			if r > t1 {
				return true
			}
			if r > t0 {
				t0 = r
			}
		} else {
			if r < t0 {
				return true
			}
			if r < t1 {
				t1 = r
			}
		}
	}
	return t0 > t1 // empty surviving interval: the segment never enters the rect
}

// pathClear reports whether a straight line from `from` to `to` stays clear
// of every wall, each inflated by pad (typically tank radius + steerMargin).
func pathClear(walls []tanks.Rect, from, to tanks.Point, pad float64) bool {
	for _, w := range walls {
		if !segmentClearsRect(from.X, from.Y, to.X, to.Y, w, pad) {
			return false
		}
	}
	return true
}

// steerAround returns the heading (radians) to drive from me toward target,
// routing around walls: the direct heading if that line is clear, otherwise
// the first clear heading among the +-30/60/90/120 degree offsets in
// steerOffsetsDeg, checked steerLookahead units ahead and preferring
// whichever side of a given offset closes more distance to target. Falls
// back to the direct heading if every offset is blocked, so a boxed-in tank
// still pushes toward its target rather than freezing.
func steerAround(walls []tanks.Rect, pad float64, me, target tanks.Point) float64 {
	direct := angleTo(me.X, me.Y, target.X, target.Y)
	if pathClear(walls, me, target, pad) {
		return direct
	}

	for _, deg := range steerOffsetsDeg {
		best, bestGain, found := direct, math.Inf(-1), false
		for _, sign := range [2]float64{1, -1} {
			heading := direct + sign*deg*math.Pi/180
			look := tanks.Point{X: me.X + steerLookahead*math.Cos(heading), Y: me.Y + steerLookahead*math.Sin(heading)}
			if !pathClear(walls, me, look, pad) {
				continue
			}
			gain := distance(me.X, me.Y, target.X, target.Y) - distance(look.X, look.Y, target.X, target.Y)
			if !found || gain > bestGain {
				best, bestGain, found = heading, gain, true
			}
		}
		if found {
			return best
		}
	}
	return direct
}

// Stuck detection: a tank commanded to move but wedged against a wall backs
// up and turns for a fixed number of ticks before resuming. hunterStrategy
// uses this shared tracker; sniperStrategy keeps its own copy inline (see
// sniper.go) since it predates this one and adds dodge/heal priorities
// around the same idea.
const (
	stuckWindowTicks  = 8
	stuckMoveThresh   = 0.2
	stuckBackoffTicks = 6
)

// stuckTracker flags a tank that was told to move but barely moved over the
// last stuckWindowTicks ticks, then walks the caller through a fixed
// reverse-and-turn recovery for stuckBackoffTicks ticks.
type stuckTracker struct {
	history     []posSample // oldest first, trimmed to stuckWindowTicks samples
	moves       []float64   // move command issued each of the last stuckWindowTicks ticks
	backoffLeft int
}

// backingOff reports whether the tracker is still inside its post-stuck
// recovery window, consuming one tick of it if so.
func (s *stuckTracker) backingOff() bool {
	if s.backoffLeft <= 0 {
		return false
	}
	s.backoffLeft--
	return true
}

// stuck reports whether, over the window recorded so far, the tank has been
// commanded to move but its position barely changed; if so it starts the
// recovery window. history holds exactly stuckWindowTicks samples once
// warmed up (the ticks immediately before this one), so its oldest entry is
// stuckWindowTicks ticks behind the current tick — the comparison this
// function needs.
func (s *stuckTracker) stuck(tick int, x, y float64) bool {
	if len(s.history) < stuckWindowTicks {
		return false
	}
	oldest := s.history[0]
	if tick-oldest.tick != stuckWindowTicks {
		return false
	}
	if distance(oldest.x, oldest.y, x, y) > stuckMoveThresh {
		return false
	}
	anyThrottle := false
	for _, m := range s.moves {
		if m != 0 {
			anyThrottle = true
			break
		}
	}
	if !anyThrottle {
		return false
	}
	s.backoffLeft = stuckBackoffTicks - 1
	return true
}

// record appends this tick's own position and issued move to the sliding
// window backingOff() and stuck() use.
func (s *stuckTracker) record(tick int, x, y, move float64) {
	s.history = append(s.history, posSample{tick: tick, x: x, y: y})
	s.moves = append(s.moves, move)
	if len(s.history) > stuckWindowTicks {
		s.history = s.history[len(s.history)-stuckWindowTicks:]
	}
	if len(s.moves) > stuckWindowTicks {
		s.moves = s.moves[len(s.moves)-stuckWindowTicks:]
	}
}
