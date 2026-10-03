package house

import (
	"hash/fnv"
	"math"
	"math/rand/v2"

	"tolerance/internal/games/tanks"
)

// profile tunes one planner-driven house strategy. The duelist, warden and
// ace differ only in which of these they switch on, so each step up the
// ladder is a strict superset of the one below it.
type profile struct {
	horizon  int  // ticks simulated per candidate plan
	twoStage bool // also try "action A for k ticks, then action B"

	// virtual: how the planner models shots that haven't been fired yet.
	// 0 = only shells already in flight; 1 = also the shot an enemy fires
	// this very tick (its turret angle is visible, so the line is known);
	// 2 = also a shot fired a few ticks from now with perfect lead aim.
	virtual int

	prefRange float64 // preferred distance to the target
	rangeBand float64 // no range cost within this many units of prefRange
	strafeW   float64 // reward for lateral movement (unpredictability)
	noise     float64 // random tie-breaking between near-equal plans
	hitW      float64 // weight of being hit

	selectTarget bool    // pick targets by HP and exposure, not just distance
	healBelow    int     // go for a heal pickup at or below this HP (0 = never)
	healEconomy  bool    // weigh the trip against the enemy getting there first
	zoneMargin   float64 // stay this far inside the zone edge
	zonePre      int     // tick at which to start drifting to the centre (0 = never)
	crossfire    float64 // weight of keeping enemies on one side
	cover        float64 // weight of hiding from enemies while reloading
	kite         bool    // back out of an enemy's effective range while reloading

	aimTol float64 // multiplier on the firing tolerance
}

type act struct{ move, turn float64 }

type seqSpec struct {
	a1, a2 act
	k      int
}

type foe struct {
	t    tanks.TankView
	v    vec // velocity, units per tick
	d    float64
	los  bool
	dpos vec
}

type vec struct{ x, y float64 }

type sshell struct {
	x, y, vx, vy float64
	minD         float64
	w            float64
	done         bool
}

type planner struct {
	prof   profile
	me     int
	rules  tanks.Rules
	walls  []tanks.Rect
	rng    *rand.Rand
	seqs   []seqSpec
	navW   int
	navH   int
	navC   []float64
	navMap map[[2]int]*navField

	prev     map[int]vec
	vel      map[int]vec
	targetID int
	strafe   float64
	stuck    stuckTracker
	dt       float64
}

// seedFromStart derives the planner's random stream from what the start
// message says (map, roster, own slot), so a given matchup replays the same
// way every time while different matchups don't all share one stream.
func seedFromStart(m tanks.StartMsg) uint64 {
	h := fnv.New64a()
	h.Write([]byte(m.Map))
	for _, pl := range m.Players {
		h.Write([]byte(pl.Name))
		h.Write([]byte{0})
	}
	h.Write([]byte{byte(m.You)})
	return h.Sum64()
}

func newPlanner(prof profile) *planner {
	return &planner{prof: prof, prev: map[int]vec{}, vel: map[int]vec{}, navMap: map[[2]int]*navField{}, strafe: 1, targetID: -1}
}

func (p *planner) Start(m tanks.StartMsg) {
	p.me = m.You
	p.rules = m.Rules
	p.walls = m.Walls
	p.dt = 1 / float64(m.Rules.TickRate)
	p.rng = rand.New(rand.NewPCG(seedFromStart(m), 0xa5a5))
	p.navW, p.navH, p.navC = navBlockedGrid(m.Rules, m.Walls)

	moves := []float64{1, 0, -1}
	turns := []float64{-1, -0.5, 0, 0.5, 1}
	var acts []act
	for _, mv := range moves {
		for _, tr := range turns {
			acts = append(acts, act{mv, tr})
		}
	}
	p.seqs = nil
	for _, a := range acts {
		p.seqs = append(p.seqs, seqSpec{a1: a, a2: a, k: 1})
	}
	if p.prof.twoStage {
		for _, a := range acts {
			for _, b := range acts {
				if a == b {
					continue
				}
				p.seqs = append(p.seqs, seqSpec{a1: a, a2: b, k: 1}, seqSpec{a1: a, a2: b, k: 3})
			}
		}
	}
}

// ---- tracking ----------------------------------------------------------

func (p *planner) track(ts []tanks.TankView) {
	for _, t := range ts {
		pos := vec{t.X, t.Y}
		if pv, ok := p.prev[t.ID]; ok && t.Alive {
			p.vel[t.ID] = vec{pos.x - pv.x, pos.y - pv.y}
		} else {
			p.vel[t.ID] = vec{}
		}
		p.prev[t.ID] = pos
	}
}

// ---- helpers -----------------------------------------------------------

func (p *planner) shellClear(x1, y1, x2, y2 float64) bool {
	for _, w := range p.walls {
		if segmentEntersRect(x1, y1, x2, y2, w, 0.05) {
			return false
		}
	}
	return true
}

func (p *planner) zoneR(tick int) float64 {
	r := p.rules
	if tick <= r.ZoneStartTick {
		return r.ZoneStartRadius
	}
	if tick >= r.ZoneEndTick {
		return r.ZoneEndRadius
	}
	f := float64(tick-r.ZoneStartTick) / float64(r.ZoneEndTick-r.ZoneStartTick)
	return r.ZoneStartRadius + f*(r.ZoneEndRadius-r.ZoneStartRadius)
}

func (p *planner) field(gx, gy float64) *navField {
	key := [2]int{int(math.Round(gx * 2)), int(math.Round(gy * 2))}
	if f, ok := p.navMap[key]; ok {
		return f
	}
	if len(p.navMap) > 80 {
		p.navMap = map[[2]int]*navField{}
	}
	f := buildNav(p.navW, p.navH, p.navC, gx, gy)
	p.navMap[key] = f
	return f
}

func pushFromRect(x, y, r float64, rect tanks.Rect) (float64, float64) {
	inside := x >= rect.X && x <= rect.X+rect.W && y >= rect.Y && y <= rect.Y+rect.H
	if inside {
		left := x - rect.X
		right := rect.X + rect.W - x
		bottom := y - rect.Y
		top := rect.Y + rect.H - y
		m := left
		axis := 0
		if right < m {
			m, axis = right, 1
		}
		if bottom < m {
			m, axis = bottom, 2
		}
		if top < m {
			axis = 3
		}
		switch axis {
		case 0:
			x = rect.X - r
		case 1:
			x = rect.X + rect.W + r
		case 2:
			y = rect.Y - r
		default:
			y = rect.Y + rect.H + r
		}
		return x, y
	}
	cx := math.Max(rect.X, math.Min(x, rect.X+rect.W))
	cy := math.Max(rect.Y, math.Min(y, rect.Y+rect.H))
	dx, dy := x-cx, y-cy
	dist := math.Hypot(dx, dy)
	if dist < r {
		if dist == 0 {
			dx, dy, dist = 1, 0, 1
		}
		x = cx + dx/dist*r
		y = cy + dy/dist*r
	}
	return x, y
}

// ---- main loop ---------------------------------------------------------

type tickCtx struct {
	tick     int
	me       tanks.TankView
	foes     []foe
	target   *foe
	shells   []sshell // real shells plus this-tick virtual ones
	goal     *navField
	goalW    float64
	rangeOn  bool
	rangeNow float64
	bonus    *vec
	zoneC    vec
}

func (p *planner) Decide(t tanks.TickMsg) tanks.CommandMsg {
	cmd := tanks.CommandMsg{Tick: t.Tick}
	me, ok := findTank(t.Tanks, p.me)
	p.track(t.Tanks)
	if !ok || !me.Alive {
		return cmd
	}

	var foes []foe
	for _, o := range t.Tanks {
		if o.ID == p.me || !o.Alive {
			continue
		}
		f := foe{t: o, v: p.vel[o.ID], d: distance(me.X, me.Y, o.X, o.Y)}
		f.los = p.shellClear(me.X, me.Y, o.X, o.Y)
		foes = append(foes, f)
	}

	c := &tickCtx{tick: t.Tick, me: me, foes: foes, zoneC: vec{t.Zone.X, t.Zone.Y}}
	c.target = p.pickTarget(me, foes)
	if c.target != nil {
		p.targetID = c.target.t.ID
	}
	c.shells = p.collectShells(c, t.Shells)
	p.chooseGoal(c, t)

	// Movement.
	best, next := p.plan(c)
	cmd.Move, cmd.Turn = best.move, best.turn
	if p.stuck.backingOff() || p.stuck.stuck(t.Tick, me.X, me.Y) {
		cmd.Move, cmd.Turn = -1, 1
		next = vec{me.X - math.Cos(me.Hull)*0.3, me.Y - math.Sin(me.Hull)*0.3}
	}
	p.stuck.record(t.Tick, me.X, me.Y, cmd.Move)

	// Turret and trigger.
	if c.target != nil {
		cmd.Turret, cmd.Fire = p.aim(c, next)
	}
	return cmd
}

func (p *planner) pickTarget(me tanks.TankView, foes []foe) *foe {
	if len(foes) == 0 {
		return nil
	}
	var best *foe
	bestS := math.Inf(1)
	for i := range foes {
		f := &foes[i]
		s := f.d
		if p.prof.selectTarget {
			s += 0.3 * float64(f.t.HP)
			if f.los {
				s -= 6
			}
			if f.t.ID == p.targetID {
				s -= 5
			}
		}
		if s < bestS {
			best, bestS = f, s
		}
	}
	return best
}

// collectShells turns the visible shells (and, depending on the profile, the
// shots enemies are about to fire) into simulation shells.
func (p *planner) collectShells(c *tickCtx, shells []tanks.ShellView) []sshell {
	var out []sshell
	for _, s := range shells {
		if s.Owner == p.me {
			continue
		}
		out = append(out, sshell{x: s.X, y: s.Y, vx: s.VX, vy: s.VY, minD: 1e9, w: 1})
	}
	if p.prof.virtual >= 1 {
		for _, f := range c.foes {
			if f.t.Reload <= 1 {
				dx, dy := math.Cos(f.t.Turret), math.Sin(f.t.Turret)
				out = append(out, sshell{
					x: f.t.X + dx*p.rules.MuzzleOffset, y: f.t.Y + dy*p.rules.MuzzleOffset,
					vx: dx * p.rules.ShellSpeed, vy: dy * p.rules.ShellSpeed, minD: 1e9, w: 0.8,
				})
			}
		}
	}
	return out
}

// chooseGoal decides what the movement score should pull toward, if anything.
func (p *planner) chooseGoal(c *tickCtx, t tanks.TickMsg) {
	me := c.me
	prof := p.prof

	// Zone drift: before the shrink starts, settle near the centre.
	if prof.zonePre > 0 && t.Tick >= prof.zonePre && distance(me.X, me.Y, c.zoneC.x, c.zoneC.y) > 10 {
		c.goal, c.goalW = p.field(c.zoneC.x, c.zoneC.y), 2.0
	}

	// Heal pickup.
	if prof.healBelow > 0 && me.HP <= prof.healBelow && len(t.Bonuses) > 0 {
		if b := p.pickBonus(c, t.Bonuses); b != nil {
			c.bonus = b
			c.goal, c.goalW = p.field(b.x, b.y), 3.0
			return
		}
	}

	if c.target != nil {
		if !c.target.los {
			// No clear line: route toward the target around the walls.
			tf := buildNav(p.navW, p.navH, p.navC, c.target.t.X, c.target.t.Y)
			c.goal, c.goalW = tf, 3.0
		} else {
			c.rangeOn = true
			c.rangeNow = prof.prefRange
			if prof.kite {
				switch {
				case c.target.t.Reload >= 6 && me.Reload <= 2:
					c.rangeNow = prof.prefRange - 4 // it just fired: close in while it reloads
				case c.target.t.Reload <= 2:
					c.rangeNow = prof.prefRange + 3 // it is ready: stay out of easy range
				}
			}
		}
	} else if c.goal == nil {
		c.goal, c.goalW = p.field(c.zoneC.x, c.zoneC.y), 2.0
	}
}

func (p *planner) pickBonus(c *tickCtx, bonuses []tanks.BonusView) *vec {
	me := c.me
	var best *vec
	bestScore := math.Inf(1)
	for _, b := range bonuses {
		f := p.field(b.X, b.Y)
		mine := f.at(me.X, me.Y)
		if mine > 45 {
			continue
		}
		if p.prof.healEconomy {
			// Skip a pickup an enemy that needs it can reach well before us.
			lost := false
			for _, e := range c.foes {
				if e.t.HP < p.rules.MaxHP && f.at(e.t.X, e.t.Y) < mine-6 {
					lost = true
				}
			}
			if lost {
				continue
			}
		}
		if mine < bestScore {
			bv := vec{b.X, b.Y}
			best, bestScore = &bv, mine
		}
	}
	return best
}

// ---- planning ----------------------------------------------------------

func (p *planner) plan(c *tickCtx) (act, vec) {
	bestCost := math.Inf(1)
	var best act
	var bestNext vec
	for _, s := range p.seqs {
		cost, next := p.eval(c, s)
		if p.prof.noise > 0 {
			cost += p.rng.Float64() * p.prof.noise
		}
		if cost < bestCost {
			bestCost, best, bestNext = cost, s.a1, next
		}
	}
	if p.rng.Float64() < 1.0/14 {
		p.strafe = -p.strafe
	}
	return best, bestNext
}

func (p *planner) eval(c *tickCtx, s seqSpec) (float64, vec) {
	r := p.rules
	prof := p.prof
	H := prof.horizon
	sub := r.Substeps
	dt := 1 / (float64(r.TickRate) * float64(sub))
	x, y, h := c.me.X, c.me.Y, c.me.Hull
	cost := 0.0

	var shells [24]sshell
	n := copy(shells[:], c.shells)
	for i := 0; i < n; i++ {
		shells[i].done = false
		shells[i].minD = 1e9
	}

	// Lead-model virtual shots: one per enemy that will be ready within the horizon.
	type vshot struct {
		at int
		f  *foe
	}
	var vshots [4]vshot
	nv := 0
	if prof.virtual >= 2 {
		for i := range c.foes {
			f := &c.foes[i]
			if f.t.Reload <= 1 {
				continue // fires this very tick: already in c.shells with its exact angle
			}
			at := f.t.Reload - 1
			if at < H-2 && nv < len(vshots) {
				vshots[nv] = vshot{at: at, f: f}
				nv++
			}
		}
	}

	px, py := x, y
	var next vec
	for i := 0; i < H; i++ {
		a := s.a1
		if i >= s.k {
			a = s.a2
		}

		// Shots fired at the start of this tick.
		for q := 0; q < nv; q++ {
			if vshots[q].at != i || n >= len(shells) {
				continue
			}
			f := vshots[q].f
			ex := f.t.X + f.v.x*float64(i)
			ey := f.t.Y + f.v.y*float64(i)
			vx, vy := (x-px)/p.dt*1, (y-py)/p.dt*1 // my velocity, units/s
			ax, ay := leadTarget(ex, ey, x, y, r.ShellSpeed, vx, vy)
			ang := math.Atan2(ay-ey, ax-ex)
			dx, dy := math.Cos(ang), math.Sin(ang)
			shells[n] = sshell{
				x: ex + dx*r.MuzzleOffset, y: ey + dy*r.MuzzleOffset,
				vx: dx * r.ShellSpeed, vy: dy * r.ShellSpeed, minD: 1e9, w: 0.55,
			}
			n++
		}
		px, py = x, y

		for ss := 0; ss < sub; ss++ {
			h = normalizeAngle(h + a.turn*r.HullTurnRate*dt)
			v := a.move * r.TankSpeed
			if a.move < 0 {
				v = a.move * r.TankReverseSpeed
			}
			x += v * dt * math.Cos(h)
			y += v * dt * math.Sin(h)
			x = math.Max(r.TankRadius, math.Min(r.Width-r.TankRadius, x))
			y = math.Max(r.TankRadius, math.Min(r.Height-r.TankRadius, y))
			for _, w := range p.walls {
				x, y = pushFromRect(x, y, r.TankRadius, w)
			}
			for k := 0; k < n; k++ {
				sh := &shells[k]
				if sh.done {
					continue
				}
				sh.x += sh.vx * dt
				sh.y += sh.vy * dt
				if sh.x < 0 || sh.x > r.Width || sh.y < 0 || sh.y > r.Height {
					sh.done = true
					continue
				}
				blocked := false
				for _, w := range p.walls {
					if sh.x >= w.X && sh.x <= w.X+w.W && sh.y >= w.Y && sh.y <= w.Y+w.H {
						blocked = true
						break
					}
				}
				if blocked {
					sh.done = true
					continue
				}
				d := math.Hypot(sh.x-x, sh.y-y)
				if d < sh.minD {
					sh.minD = d
				}
				if d <= r.TankRadius+0.05 {
					cost += prof.hitW * sh.w
					sh.done = true
					sh.minD = 1e9
				}
			}
		}
		if i == 0 {
			next = vec{x, y}
		}

		// Per-tick terms: zone, enemy proximity.
		zr := p.zoneR(c.tick+i) - prof.zoneMargin
		if zd := math.Hypot(x-c.zoneC.x, y-c.zoneC.y); zd > zr {
			cost += 3 * (zd - zr)
		}
		for _, f := range c.foes {
			ex := f.t.X + f.v.x*float64(i+1)
			ey := f.t.Y + f.v.y*float64(i+1)
			if d := math.Hypot(x-ex, y-ey); d < 2.6 {
				cost += 10 * (2.6 - d)
			}
		}
	}

	// Near misses.
	for k := 0; k < n; k++ {
		sh := &shells[k]
		if sh.minD < 1.9 {
			cost += prof.hitW * sh.w * 0.25 * (1.9 - sh.minD) / 0.85
		}
	}

	// End-of-horizon terms.
	if c.goal != nil {
		cost += c.goalW * c.goal.at(x, y)
	}
	if c.bonus != nil {
		// nothing extra: the nav field already pulls toward it
	}
	if t := c.target; t != nil && c.rangeOn {
		ex := t.t.X + t.v.x*float64(H)
		ey := t.t.Y + t.v.y*float64(H)
		d := math.Hypot(x-ex, y-ey)
		dev := math.Abs(d-c.rangeNow) - prof.rangeBand
		if dev > 0 {
			cost += 3 * dev
		}
		bearing := math.Atan2(ey-y, ex-x)
		cs := math.Cos(angleDiff(h, bearing))
		if dev <= 3 {
			// Keep the hull square to the enemy: forward/back is then a sideways dodge.
			cost += 6 * cs * cs
		}
		// Lateral movement, in the current strafe direction.
		dx, dy := x-c.me.X, y-c.me.Y
		lat := -math.Sin(bearing)*dx*p.strafe + math.Cos(bearing)*dy*p.strafe
		cost -= prof.strafeW * lat
	}
	if prof.cover > 0 {
		// Third parties: stay out of the line of fire of anyone who can
		// shoot soon and isn't who we are fighting.
		for i := range c.foes {
			f := &c.foes[i]
			if f == c.target || f.t.Reload > 3 {
				continue
			}
			ex := f.t.X + f.v.x*float64(H)
			ey := f.t.Y + f.v.y*float64(H)
			if p.shellClear(x, y, ex, ey) {
				cost += prof.cover
			}
		}
	}
	// Wall / edge hugging.
	ci := clampInt(int(x), 0, p.navW-1)
	cj := clampInt(int(y), 0, p.navH-1)
	cost += 1.5 * (p.navC[cj*p.navW+ci] - 1)

	if prof.crossfire > 0 && len(c.foes) >= 2 {
		for i := 0; i < len(c.foes); i++ {
			for j := i + 1; j < len(c.foes); j++ {
				a1 := math.Atan2(c.foes[i].t.Y-y, c.foes[i].t.X-x)
				a2 := math.Atan2(c.foes[j].t.Y-y, c.foes[j].t.X-x)
				cost += prof.crossfire * (1 - math.Cos(angleDiff(a1, a2))) / 2
			}
		}
	}

	return cost, next
}

// ---- aiming ------------------------------------------------------------

func (p *planner) aim(c *tickCtx, myNext vec) (turret float64, fire bool) {
	r := p.rules
	me, t := c.me, c.target
	vps := vec{t.v.x / p.dt, t.v.y / p.dt}

	// Where the shell would meet the target if fired now.
	mx := me.X + math.Cos(me.Turret)*r.MuzzleOffset
	my := me.Y + math.Sin(me.Turret)*r.MuzzleOffset
	ax, ay := leadTarget(mx, my, t.t.X, t.t.Y, r.ShellSpeed, vps.x, vps.y)
	ax = math.Max(0.3, math.Min(r.Width-0.3, ax))
	ay = math.Max(0.3, math.Min(r.Height-0.3, ay))
	ang := math.Atan2(ay-me.Y, ax-me.X)
	d := math.Hypot(ax-me.X, ay-me.Y)
	tol := math.Atan2(0.85, math.Max(d, 1.5)) * p.prof.aimTol
	diff := angleDiff(ang, me.Turret)
	fire = math.Abs(diff) < tol && p.shellClear(mx, my, ax, ay)

	// Turret command for next tick: aim at the predicted situation then.
	tx, ty := t.t.X+t.v.x, t.t.Y+t.v.y
	nmx := myNext.x + math.Cos(ang)*r.MuzzleOffset
	nmy := myNext.y + math.Sin(ang)*r.MuzzleOffset
	nax, nay := leadTarget(nmx, nmy, tx, ty, r.ShellSpeed, vps.x, vps.y)
	nang := math.Atan2(nay-myNext.y, nax-myNext.x)
	ndiff := angleDiff(nang, me.Turret)
	turret = clamp1(ndiff / (r.TurretTurnRate * p.dt))
	return turret, fire
}
