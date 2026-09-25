package house

import (
	"math"

	"tolerance/internal/games/tanks"
)

// hunterStrategy charges the nearest living enemy and fires once its
// turret is aimed. It's the simplest strategy that isn't idle, and the
// starter kits' bot.py/bot.js ship the same strategy as a starting point.
type hunterStrategy struct {
	me int
}

func (s *hunterStrategy) Start(m tanks.StartMsg) {
	s.me = m.You
}

func (s *hunterStrategy) Decide(t tanks.TickMsg) tanks.CommandMsg {
	cmd := tanks.CommandMsg{Tick: t.Tick}

	me, ok := findTank(t.Tanks, s.me)
	if !ok || !me.Alive {
		return cmd
	}

	target := nearestEnemy(me, t.Tanks)
	if target == nil {
		return cmd
	}

	targetAngle := angleTo(me.X, me.Y, target.X, target.Y)

	hullDiff := angleDiff(targetAngle, me.Hull)
	cmd.Turn = clamp1(3 * hullDiff)

	dist := distance(me.X, me.Y, target.X, target.Y)
	if dist > 6 {
		cmd.Move = 1
	}

	turretDiff := angleDiff(targetAngle, me.Turret)
	cmd.Turret = clamp1(4 * turretDiff)
	cmd.Fire = math.Abs(turretDiff) < 0.08

	return cmd
}
