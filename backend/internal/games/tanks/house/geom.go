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
