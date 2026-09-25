package tanks

import (
	"math"
	"math/rand/v2"
)

// Command is one tick's input for one tank. Move, Turn and Turret are
// clamped to [-1, 1] before use.
type Command struct {
	Move   float64
	Turn   float64
	Turret float64
	Fire   bool
}

// Tank is one player's tank. ID always equals its index in Game.Tanks and
// never changes.
type Tank struct {
	ID        int
	X, Y      float64
	Hull      float64
	Turret    float64
	HP        int
	Reload    int
	Alive     bool
	Kills     int
	Damage    int
	DeathTick int // -1 while alive
}

// Shell is a projectile in flight.
type Shell struct {
	ID, Owner int
	X, Y      float64
	VX, VY    float64
	Age       int
}

// Bonus is a health pickup location.
type Bonus struct {
	X, Y      float64
	Active    bool
	RespawnAt int
}

// Event is one thing that happened on a tick.
//
//   - "shot": A fired.
//   - "hit": A hit B for D damage.
//   - "kill": A killed B; A == -1 means the shrinking zone killed B.
//   - "heal": A picked up a bonus, healing D HP.
type Event struct {
	T int    `json:"t"`
	E string `json:"e"`
	A int    `json:"a"`
	B *int   `json:"b,omitempty"`
	D *int   `json:"d,omitempty"`
}

// Game is one match's mutable simulation state.
type Game struct {
	Rules   Rules
	Map     Map
	Seed    int64
	Tick    int
	Tanks   []Tank
	Shells  []Shell
	Bonuses []Bonus
	Zone    Zone

	nextShellID int
	rng         *rand.Rand
}

// New starts a match for 2..4 players on m. Slots get spawn points: 2
// players use spawns 0 and 2, 3 players use 0,1,2, 4 players use all four;
// the order of the points used is shuffled by a PCG rng seeded from seed.
// Hull and turret start pointed at the field centre. All bonuses start
// active.
func New(rules Rules, m Map, players int, seed int64) *Game {
	g := &Game{
		Rules: rules,
		Map:   m,
		Seed:  seed,
		Tick:  0,
		rng:   rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9e3779b97f4a7c15)),
	}

	var slots []int
	switch players {
	case 2:
		slots = []int{0, 2}
	case 3:
		slots = []int{0, 1, 2}
	default:
		slots = []int{0, 1, 2, 3}
	}
	points := make([]Point, len(slots))
	for i, si := range slots {
		points[i] = m.Spawns[si]
	}
	g.rng.Shuffle(len(points), func(i, j int) { points[i], points[j] = points[j], points[i] })

	cx, cy := rules.Width/2, rules.Height/2
	g.Tanks = make([]Tank, players)
	for i := 0; i < players; i++ {
		p := points[i]
		angle := math.Atan2(cy-p.Y, cx-p.X)
		g.Tanks[i] = Tank{
			ID:        i,
			X:         p.X,
			Y:         p.Y,
			Hull:      angle,
			Turret:    angle,
			HP:        rules.MaxHP,
			Alive:     true,
			DeathTick: -1,
		}
	}

	g.Bonuses = make([]Bonus, len(m.Bonuses))
	for i, p := range m.Bonuses {
		g.Bonuses[i] = Bonus{X: p.X, Y: p.Y, Active: true}
	}

	g.Zone = Zone{X: cx, Y: cy, R: rules.ZoneStartRadius}
	return g
}

// Over reports whether the match has ended: the tick limit was reached, or
// at most one tank is alive.
func (g *Game) Over() bool {
	return g.Tick >= g.Rules.Ticks || g.Alive() <= 1
}

// Alive returns the number of tanks currently alive.
func (g *Game) Alive() int {
	n := 0
	for _, t := range g.Tanks {
		if t.Alive {
			n++
		}
	}
	return n
}

func clamp1(v float64) float64 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// normalizeAngle brings a into (-pi, pi].
func normalizeAngle(a float64) float64 {
	a = math.Mod(a, 2*math.Pi)
	if a <= -math.Pi {
		a += 2 * math.Pi
	} else if a > math.Pi {
		a -= 2 * math.Pi
	}
	return a
}

func intPtr(v int) *int { return &v }

// pushFromEdges clamps a tank centre back inside the field boundary.
func pushFromEdges(x, y, r, width, height float64) (float64, float64) {
	x = clampf(x, r, width-r)
	y = clampf(y, r, height-r)
	return x, y
}

// pushFromRect resolves a circle-vs-rectangle overlap: the circle is pushed
// out along the normal from the rectangle's closest point, or, if its
// centre is inside the rectangle, out along the shortest axis.
func pushFromRect(x, y, r float64, rect Rect) (float64, float64) {
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
			m, axis = top, 3
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

	cx := clampf(x, rect.X, rect.X+rect.W)
	cy := clampf(y, rect.Y, rect.Y+rect.H)
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

// zoneRadius is the shrinking zone's radius at tick.
func zoneRadius(rules Rules, tick int) float64 {
	if tick <= rules.ZoneStartTick {
		return rules.ZoneStartRadius
	}
	if tick >= rules.ZoneEndTick {
		return rules.ZoneEndRadius
	}
	frac := float64(tick-rules.ZoneStartTick) / float64(rules.ZoneEndTick-rules.ZoneStartTick)
	return rules.ZoneStartRadius + frac*(rules.ZoneEndRadius-rules.ZoneStartRadius)
}

// Step advances the game by one tick and returns the events it produced.
// len(cmds) must equal len(g.Tanks); commands for dead tanks are ignored.
func (g *Game) Step(cmds []Command) []Event {
	tick := g.Tick
	rules := g.Rules
	events := []Event{}

	// 1. Reload and firing, in tank ID order.
	for i := range g.Tanks {
		t := &g.Tanks[i]
		if !t.Alive {
			continue
		}
		cmd := cmds[i]
		if t.Reload > 0 {
			t.Reload--
		}
		if cmd.Fire && t.Reload == 0 {
			mx := t.X + math.Cos(t.Turret)*rules.MuzzleOffset
			my := t.Y + math.Sin(t.Turret)*rules.MuzzleOffset
			g.Shells = append(g.Shells, Shell{
				ID:    g.nextShellID,
				Owner: t.ID,
				X:     mx,
				Y:     my,
				VX:    math.Cos(t.Turret) * rules.ShellSpeed,
				VY:    math.Sin(t.Turret) * rules.ShellSpeed,
			})
			g.nextShellID++
			t.Reload = rules.ReloadTicks
			events = append(events, Event{T: tick, E: "shot", A: t.ID})
		}
	}

	// 2. Physics substeps: move tanks, resolve collisions, move shells.
	dt := 1 / (float64(rules.TickRate) * float64(rules.Substeps))
	for s := 0; s < rules.Substeps; s++ {
		for i := range g.Tanks {
			t := &g.Tanks[i]
			if !t.Alive {
				continue
			}
			cmd := cmds[i]
			turn := clamp1(cmd.Turn)
			turretCmd := clamp1(cmd.Turret)
			move := clamp1(cmd.Move)

			t.Hull = normalizeAngle(t.Hull + turn*rules.HullTurnRate*dt)
			t.Turret = normalizeAngle(t.Turret + turretCmd*rules.TurretTurnRate*dt)

			var v float64
			if move >= 0 {
				v = move * rules.TankSpeed
			} else {
				v = move * rules.TankReverseSpeed
			}
			t.X += v * dt * math.Cos(t.Hull)
			t.Y += v * dt * math.Sin(t.Hull)
		}

		for i := range g.Tanks {
			t := &g.Tanks[i]
			if !t.Alive {
				continue
			}
			t.X, t.Y = pushFromEdges(t.X, t.Y, rules.TankRadius, rules.Width, rules.Height)
			for _, w := range g.Map.Walls {
				t.X, t.Y = pushFromRect(t.X, t.Y, rules.TankRadius, w)
			}
		}

		for i := range g.Tanks {
			ti := &g.Tanks[i]
			if !ti.Alive {
				continue
			}
			for j := i + 1; j < len(g.Tanks); j++ {
				tj := &g.Tanks[j]
				if !tj.Alive {
					continue
				}
				dx, dy := tj.X-ti.X, tj.Y-ti.Y
				d := math.Hypot(dx, dy)
				minGap := 2 * rules.TankRadius
				if d < minGap {
					var nx, ny float64
					if d == 0 {
						nx, ny = 1, 0
					} else {
						nx, ny = dx/d, dy/d
					}
					push := (minGap - d) / 2
					ti.X -= nx * push
					ti.Y -= ny * push
					tj.X += nx * push
					tj.Y += ny * push
				}
			}
		}

		remaining := g.Shells[:0]
		for _, sh := range g.Shells {
			sh.X += sh.VX * dt
			sh.Y += sh.VY * dt

			if sh.X < 0 || sh.X > rules.Width || sh.Y < 0 || sh.Y > rules.Height {
				continue
			}
			hitWall := false
			for _, w := range g.Map.Walls {
				if sh.X >= w.X && sh.X <= w.X+w.W && sh.Y >= w.Y && sh.Y <= w.Y+w.H {
					hitWall = true
					break
				}
			}
			if hitWall {
				continue
			}

			hit := -1
			for i := range g.Tanks {
				t := &g.Tanks[i]
				if !t.Alive || t.ID == sh.Owner {
					continue
				}
				if math.Hypot(t.X-sh.X, t.Y-sh.Y) <= rules.TankRadius {
					hit = i
					break
				}
			}
			if hit >= 0 {
				target := &g.Tanks[hit]
				dmg := min(rules.ShellDamage, target.HP)
				target.HP -= dmg
				g.Tanks[sh.Owner].Damage += dmg
				events = append(events, Event{T: tick, E: "hit", A: sh.Owner, B: intPtr(target.ID), D: intPtr(dmg)})
				if target.HP <= 0 {
					target.Alive = false
					target.DeathTick = tick
					g.Tanks[sh.Owner].Kills++
					events = append(events, Event{T: tick, E: "kill", A: sh.Owner, B: intPtr(target.ID)})
				}
				continue
			}

			remaining = append(remaining, sh)
		}
		g.Shells = remaining
	}

	// 3. Age shells and expire them.
	remaining := g.Shells[:0]
	for _, sh := range g.Shells {
		sh.Age++
		if sh.Age >= rules.ShellLifetimeTicks {
			continue
		}
		remaining = append(remaining, sh)
	}
	g.Shells = remaining

	// 4. Bonus pickups and respawns.
	for bi := range g.Bonuses {
		b := &g.Bonuses[bi]
		if b.Active {
			for i := range g.Tanks {
				t := &g.Tanks[i]
				if !t.Alive || t.HP >= rules.MaxHP {
					continue
				}
				if math.Hypot(t.X-b.X, t.Y-b.Y) < rules.PickupRadius {
					before := t.HP
					t.HP = min(rules.MaxHP, t.HP+rules.HealAmount)
					gain := t.HP - before
					events = append(events, Event{T: tick, E: "heal", A: t.ID, D: intPtr(gain)})
					b.Active = false
					b.RespawnAt = tick + rules.HealRespawnTicks
					break
				}
			}
		} else if tick >= b.RespawnAt {
			b.Active = true
		}
	}

	// 5. Shrinking zone.
	r := zoneRadius(rules, tick)
	cx, cy := rules.Width/2, rules.Height/2
	g.Zone = Zone{X: cx, Y: cy, R: r}
	for i := range g.Tanks {
		t := &g.Tanks[i]
		if !t.Alive {
			continue
		}
		if math.Hypot(t.X-cx, t.Y-cy) > r {
			dmg := min(rules.ZoneDamagePerTick, t.HP)
			t.HP -= dmg
			if t.HP <= 0 {
				t.Alive = false
				t.DeathTick = tick
				events = append(events, Event{T: tick, E: "kill", A: -1, B: intPtr(t.ID)})
			}
		}
	}

	// 6. Advance the tick.
	g.Tick = tick + 1

	return events
}
