package house

import (
	"math"

	"tolerance/internal/games/tanks"
)

// hunterStrategy charges the nearest living enemy, routing around walls in
// between and backing off if it gets wedged against one, and fires once its
// turret is aimed and the line of fire is clear. It's the simplest strategy
// that isn't idle, and the starter kits' bot.py/bot.js ship the same
// strategy as a starting point.
type hunterStrategy struct {
	me    int
	walls []tanks.Rect
	pad   float64 // tank radius + steerMargin, precomputed once in Start
	stuck stuckTracker
}

func (s *hunterStrategy) Start(m tanks.StartMsg) {
	s.me = m.You
	s.walls = m.Walls
	s.pad = m.Rules.TankRadius + steerMargin
}

func (s *hunterStrategy) Decide(t tanks.TickMsg) tanks.CommandMsg {
	cmd := tanks.CommandMsg{Tick: t.Tick}

	me, ok := findTank(t.Tanks, s.me)
	if !ok || !me.Alive {
		return cmd
	}

	// Wedged against a wall takes priority over everything else: back off
	// with a turn for a fixed number of ticks, then resume.
	if s.stuck.backingOff() {
		cmd.Move, cmd.Turn = -1, 1
		s.stuck.record(t.Tick, me.X, me.Y, cmd.Move)
		return cmd
	}
	if s.stuck.stuck(t.Tick, me.X, me.Y) {
		cmd.Move, cmd.Turn = -1, 1
		s.stuck.record(t.Tick, me.X, me.Y, cmd.Move)
		return cmd
	}

	target := nearestEnemy(me, t.Tanks)
	if target == nil {
		s.stuck.record(t.Tick, me.X, me.Y, cmd.Move)
		return cmd
	}

	mePt := tanks.Point{X: me.X, Y: me.Y}
	targetPt := tanks.Point{X: target.X, Y: target.Y}
	lineClear := pathClear(s.walls, mePt, targetPt, s.pad)

	// Steer the hull toward the target, routing around any wall in
	// between instead of driving straight into it.
	heading := steerAround(s.walls, s.pad, mePt, targetPt, lineClear)
	hullDiff := angleDiff(heading, me.Hull)
	cmd.Turn = clamp1(3 * hullDiff)

	// Keep closing until both close range AND a clear line of fire: being
	// within 6 units through a wall corner isn't "close enough" — it's a
	// standoff, since a blocked line never lets fire trigger below.
	dist := distance(me.X, me.Y, target.X, target.Y)
	if dist > 6 || !lineClear {
		cmd.Move = 1
	}

	// Aim the turret straight at the target (not the steering heading),
	// and only fire when the line of fire is clear of walls -- no point
	// spending a reload on cover.
	targetAngle := angleTo(me.X, me.Y, target.X, target.Y)
	turretDiff := angleDiff(targetAngle, me.Turret)
	cmd.Turret = clamp1(4 * turretDiff)
	cmd.Fire = math.Abs(turretDiff) < 0.08 && lineClear

	s.stuck.record(t.Tick, me.X, me.Y, cmd.Move)
	return cmd
}
