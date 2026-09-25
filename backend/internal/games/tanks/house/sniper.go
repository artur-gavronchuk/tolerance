package house

import (
	"math"

	"tolerance/internal/games/tanks"
)

const (
	sniperMinRange     = 14.0
	sniperMaxRange     = 20.0
	sniperAimTolerance = 0.08
	sniperLowHP        = 45
	sniperDodgeRadius  = 1.5
	sniperDodgeTicks   = 10
	sniperStuckTicks   = 5
	sniperStuckMove    = 0.2
	sniperUnstickTicks = 5
)

// posSample is one tick's recorded position, used both for the sniper's
// own stuck-at-wall detection and for estimating an enemy tank's velocity
// from its last couple of positions.
type posSample struct {
	tick int
	x, y float64
}

// sniperStrategy keeps its distance from the nearest enemy, leads its
// shots using the target's estimated velocity, dodges incoming shells, and
// backs off with a turn if it gets stuck against a wall.
type sniperStrategy struct {
	me       int
	tickRate int
	shellV   float64

	// selfHistory holds the tank's own position every tick, oldest first,
	// trimmed to sniperStuckTicks+1 samples: enough to compare "now"
	// against "sniperStuckTicks ticks ago".
	selfHistory []posSample
	// selfMoves holds the move command issued each of the last
	// sniperStuckTicks ticks, aligned with selfHistory.
	selfMoves []float64

	// targetPrev is the last tick's position of whichever enemy tank we
	// are currently tracking, used to estimate its velocity.
	targetPrev   posSample
	targetPrevOK bool

	unstickTicksLeft int
}

func (s *sniperStrategy) Start(m tanks.StartMsg) {
	s.me = m.You
	s.tickRate = m.Rules.TickRate
	if s.tickRate <= 0 {
		s.tickRate = 10
	}
	s.shellV = m.Rules.ShellSpeed
}

func (s *sniperStrategy) Decide(t tanks.TickMsg) tanks.CommandMsg {
	cmd := tanks.CommandMsg{Tick: t.Tick}

	me, ok := findTank(t.Tanks, s.me)
	if !ok || !me.Alive {
		return cmd
	}

	// Stuck-at-a-wall recovery takes priority over everything else: back
	// off with a turn for a fixed number of ticks, then resume.
	if s.unstickTicksLeft > 0 {
		s.unstickTicksLeft--
		cmd.Move = -1
		cmd.Turn = 1
		s.recordSelf(t.Tick, me.X, me.Y, cmd.Move)
		return cmd
	}
	if s.stuck(t.Tick, me.X, me.Y) {
		s.unstickTicksLeft = sniperUnstickTicks - 1
		cmd.Move = -1
		cmd.Turn = 1
		s.recordSelf(t.Tick, me.X, me.Y, cmd.Move)
		return cmd
	}

	target := nearestEnemy(me, t.Tanks)

	dt := 1 / float64(s.tickRate)

	// Pick the one thing worth moving for this tick, most urgent first:
	// dodge an incoming shell, go heal if critically low, or hold
	// distance from the target. All of them drive at full throttle in a
	// chosen heading — reversing away from a chaser at the slower reverse
	// speed just lets it close the gap, so "too close" backs off by
	// turning away and driving forward instead.
	heading, moving := 0.0, false
	if dodgeHeading, dodging := s.dodgeAngle(me, t.Shells, dt); dodging {
		heading, moving = dodgeHeading, true
	} else if bonus := nearestBonus(me, t.Bonuses); me.HP < sniperLowHP && bonus != nil {
		heading, moving = angleTo(me.X, me.Y, bonus.X, bonus.Y), true
	} else if target != nil {
		dist := distance(me.X, me.Y, target.X, target.Y)
		switch {
		case dist < sniperMinRange:
			heading, moving = angleTo(target.X, target.Y, me.X, me.Y), true // face away, drive off
		case dist > sniperMaxRange:
			heading, moving = angleTo(me.X, me.Y, target.X, target.Y), true // face in, close the gap
		}
	}
	if moving {
		diff := angleDiff(heading, me.Hull)
		cmd.Turn = clamp1(3 * diff)
		cmd.Move = 1
	}

	if target != nil {
		var vx, vy float64
		if s.targetPrevOK && s.targetPrev.tick == t.Tick-1 {
			vx = (target.X - s.targetPrev.x) / dt
			vy = (target.Y - s.targetPrev.y) / dt
		}
		s.targetPrev = posSample{tick: t.Tick, x: target.X, y: target.Y}
		s.targetPrevOK = true

		aimX, aimY := leadTarget(me.X, me.Y, target.X, target.Y, s.shellV, vx, vy)
		turretAngle := angleTo(me.X, me.Y, aimX, aimY)
		turretDiff := angleDiff(turretAngle, me.Turret)
		cmd.Turret = clamp1(4 * turretDiff)
		cmd.Fire = math.Abs(turretDiff) < sniperAimTolerance
	}

	s.recordSelf(t.Tick, me.X, me.Y, cmd.Move)
	return cmd
}

// recordSelf appends this tick's own position and issued move to the
// sliding window used by stuck().
func (s *sniperStrategy) recordSelf(tick int, x, y, move float64) {
	s.selfHistory = append(s.selfHistory, posSample{tick: tick, x: x, y: y})
	s.selfMoves = append(s.selfMoves, move)
	if len(s.selfHistory) > sniperStuckTicks+1 {
		s.selfHistory = s.selfHistory[len(s.selfHistory)-(sniperStuckTicks+1):]
	}
	if len(s.selfMoves) > sniperStuckTicks {
		s.selfMoves = s.selfMoves[len(s.selfMoves)-sniperStuckTicks:]
	}
}

// stuck reports whether, over the window recorded so far, the tank has
// been commanded to move but its position barely changed — the wall-jam
// case the brief calls out.
func (s *sniperStrategy) stuck(tick int, x, y float64) bool {
	if len(s.selfHistory) <= sniperStuckTicks {
		return false
	}
	oldest := s.selfHistory[0]
	if tick-oldest.tick != sniperStuckTicks {
		return false
	}
	moved := distance(oldest.x, oldest.y, x, y)
	if moved > sniperStuckMove {
		return false
	}
	anyThrottle := false
	for _, m := range s.selfMoves {
		if m != 0 {
			anyThrottle = true
			break
		}
	}
	return anyThrottle
}

// dodgeAngle looks for an enemy shell that will pass within
// sniperDodgeRadius of me in the next sniperDodgeTicks ticks and, if found,
// returns the hull heading that steers away from its line of flight.
func (s *sniperStrategy) dodgeAngle(me tanks.TankView, shells []tanks.ShellView, dt float64) (float64, bool) {
	maxSeconds := float64(sniperDodgeTicks) * dt
	for _, sh := range shells {
		if sh.Owner == me.ID {
			continue
		}
		if shellClosestApproach(sh.X, sh.Y, sh.VX, sh.VY, me.X, me.Y, maxSeconds) >= sniperDodgeRadius {
			continue
		}
		shellDir := math.Atan2(sh.VY, sh.VX)
		cross := sh.VX*(me.Y-sh.Y) - sh.VY*(me.X-sh.X)
		if cross >= 0 {
			return normalizeAngle(shellDir + math.Pi/2), true
		}
		return normalizeAngle(shellDir - math.Pi/2), true
	}
	return 0, false
}

// nearestBonus returns the closest active bonus to me, or nil.
func nearestBonus(me tanks.TankView, bonuses []tanks.BonusView) *tanks.BonusView {
	var best *tanks.BonusView
	bestD := math.Inf(1)
	for i := range bonuses {
		b := bonuses[i]
		if d := distance(me.X, me.Y, b.X, b.Y); d < bestD {
			bestD = d
			cp := b
			best = &cp
		}
	}
	return best
}
