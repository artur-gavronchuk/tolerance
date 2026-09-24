package tanks

import (
	"math"
	"math/rand/v2"
	"reflect"
	"testing"
)

func newTestGame(t *testing.T, walls []Rect, pos []Point) *Game {
	t.Helper()
	m := Map{
		Name:    "t",
		Walls:   walls,
		Spawns:  [4]Point{{X: 5, Y: 5}, {X: 5, Y: 35}, {X: 55, Y: 35}, {X: 55, Y: 5}},
		Bonuses: [4]Point{{X: 20, Y: 20}, {X: 30, Y: 30}, {X: 40, Y: 20}, {X: 30, Y: 10}},
	}
	g := New(DefaultRules(), m, len(pos), 1)
	for i, p := range pos {
		g.Tanks[i].X = p.X
		g.Tanks[i].Y = p.Y
	}
	for i := range g.Bonuses {
		g.Bonuses[i].Active = false
		g.Bonuses[i].RespawnAt = 1 << 30
	}
	return g
}

func approx(t *testing.T, name string, got, want, eps float64) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s = %v, want ~%v (eps %v)", name, got, want, eps)
	}
}

func TestMoveForwardOneTick(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 10, Y: 10}, {X: 0, Y: 0}})
	g.Tanks[0].Hull = 0
	g.Step([]Command{{Move: 1}, {}})
	approx(t, "X", g.Tanks[0].X, 10.5, 1e-9)
	approx(t, "Y", g.Tanks[0].Y, 10, 1e-9)
}

func TestReverseIsSlower(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 10, Y: 10}, {X: 0, Y: 0}})
	g.Tanks[0].Hull = 0
	g.Step([]Command{{Move: -1}, {}})
	approx(t, "X", g.Tanks[0].X, 9.7, 1e-9)
}

func TestTurnRates(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 10, Y: 10}, {X: 0, Y: 0}})
	g.Tanks[0].Hull = 0
	g.Tanks[0].Turret = 0
	g.Step([]Command{{Turn: 1, Turret: -1}, {}})
	approx(t, "Hull", g.Tanks[0].Hull, 0.25, 1e-9)
	approx(t, "Turret", g.Tanks[0].Turret, -0.4, 1e-9)
}

func TestCommandsClamped(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 10, Y: 10}, {X: 0, Y: 0}})
	g.Tanks[0].Hull = 0
	g.Step([]Command{{Move: 7}, {}})
	approx(t, "X", g.Tanks[0].X, 10.5, 1e-9)
}

func TestWallStopsTank(t *testing.T) {
	walls := []Rect{{X: 12, Y: 0, W: 2, H: 40}}
	g := newTestGame(t, walls, []Point{{X: 10.8, Y: 20}, {X: 0, Y: 0}})
	g.Tanks[0].Hull = 0
	for i := 0; i < 5; i++ {
		g.Step([]Command{{Move: 1}, {}})
	}
	if g.Tanks[0].X > 12-1.0+1e-9 {
		t.Errorf("X = %v, want <= %v", g.Tanks[0].X, 12-1.0+1e-9)
	}
}

func TestFieldEdgeStopsTank(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 1.2, Y: 20}, {X: 30, Y: 30}})
	g.Tanks[0].Hull = math.Pi
	for i := 0; i < 5; i++ {
		g.Step([]Command{{Move: 1}, {}})
	}
	if g.Tanks[0].X < 1.0 {
		t.Errorf("X = %v, want >= 1.0", g.Tanks[0].X)
	}
}

func TestTanksPushApart(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 10, Y: 10}, {X: 11, Y: 10}})
	g.Step([]Command{{}, {}})
	d := math.Hypot(g.Tanks[1].X-g.Tanks[0].X, g.Tanks[1].Y-g.Tanks[0].Y)
	if d < 2.0-1e-9 {
		t.Errorf("distance = %v, want >= %v", d, 2.0-1e-9)
	}
}

func TestFireSpawnsShellAndReloads(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 10, Y: 10}, {X: 0, Y: 0}})
	events := g.Step([]Command{{Fire: true}, {}})
	if len(g.Shells) != 1 {
		t.Fatalf("len(Shells) = %d, want 1", len(g.Shells))
	}
	if g.Tanks[0].Reload != 10 {
		t.Errorf("Reload = %d, want 10", g.Tanks[0].Reload)
	}
	if !hasEvent(events, "shot", 0) {
		t.Errorf("events = %+v, want a shot event", events)
	}

	events = g.Step([]Command{{Fire: true}, {}})
	if len(g.Shells) != 1 {
		t.Fatalf("len(Shells) = %d, want 1 (no new shell)", len(g.Shells))
	}
	if g.Tanks[0].Reload != 9 {
		t.Errorf("Reload = %d, want 9", g.Tanks[0].Reload)
	}
	if hasEvent(events, "shot", 0) {
		t.Errorf("events = %+v, want no shot event", events)
	}
}

func hasEvent(events []Event, e string, a int) bool {
	for _, ev := range events {
		if ev.E == e && ev.A == a {
			return true
		}
	}
	return false
}

func TestShellHitsAndKills(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 10, Y: 20}, {X: 20, Y: 20}})
	g.Tanks[0].Hull, g.Tanks[0].Turret = 0, 0
	g.Tanks[1].Hull, g.Tanks[1].Turret = math.Pi, math.Pi

	hits, kills := 0, 0
	for i := 0; i < 200 && g.Tanks[1].Alive; i++ {
		events := g.Step([]Command{{Fire: true}, {}})
		for _, ev := range events {
			switch ev.E {
			case "hit":
				hits++
			case "kill":
				kills++
				if ev.A != 0 || ev.B == nil || *ev.B != 1 {
					t.Errorf("kill event = %+v, want A=0 B=1", ev)
				}
			}
		}
	}

	if g.Tanks[1].Alive {
		t.Fatal("tank 1 should be dead")
	}
	if g.Tanks[1].DeathTick < 0 {
		t.Errorf("DeathTick = %d, want >= 0", g.Tanks[1].DeathTick)
	}
	if g.Tanks[0].Kills != 1 {
		t.Errorf("Kills = %d, want 1", g.Tanks[0].Kills)
	}
	if g.Tanks[0].Damage != 100 {
		t.Errorf("Damage = %d, want 100", g.Tanks[0].Damage)
	}
	if hits != 4 {
		t.Errorf("hits = %d, want 4", hits)
	}
	if kills != 1 {
		t.Errorf("kills = %d, want 1", kills)
	}
}

func TestShellDoesNotHitOwner(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 10, Y: 20}, {X: 30, Y: 30}})
	g.Tanks[0].Hull, g.Tanks[0].Turret = 0, 0
	for i := 0; i < 10; i++ {
		g.Step([]Command{{Fire: true}, {}})
	}
	if g.Tanks[0].HP != g.Rules.MaxHP {
		t.Errorf("HP = %d, want %d (own shells must not hit self)", g.Tanks[0].HP, g.Rules.MaxHP)
	}
}

func TestShellStoppedByWall(t *testing.T) {
	walls := []Rect{{X: 15, Y: 0, W: 4, H: 40}}
	g := newTestGame(t, walls, []Point{{X: 10, Y: 20}, {X: 20, Y: 20}})
	g.Tanks[0].Hull, g.Tanks[0].Turret = 0, 0
	for i := 0; i < 20; i++ {
		g.Step([]Command{{Fire: true}, {}})
	}
	if g.Tanks[1].HP != g.Rules.MaxHP {
		t.Errorf("HP = %d, want %d (wall should stop the shell)", g.Tanks[1].HP, g.Rules.MaxHP)
	}
	if len(g.Shells) != 0 {
		t.Errorf("len(Shells) = %d, want 0", len(g.Shells))
	}
}

func TestShellExpires(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 30, Y: 20}, {X: 0, Y: 0}})
	g.Tanks[0].Hull, g.Tanks[0].Turret = 0, 0
	g.Step([]Command{{Fire: true}, {}})
	if len(g.Shells) != 1 {
		t.Fatalf("len(Shells) = %d, want 1", len(g.Shells))
	}
	for i := 0; i < 40; i++ {
		g.Step([]Command{{}, {}})
	}
	if len(g.Shells) != 0 {
		t.Errorf("len(Shells) = %d, want 0 (shell should have expired)", len(g.Shells))
	}
}

func TestHealPickup(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 20, Y: 20}, {X: 40, Y: 20}, {X: 0, Y: 0}})
	g.Tanks[0].HP = 50
	g.Tanks[1].HP = 100
	g.Bonuses[0].X, g.Bonuses[0].Y, g.Bonuses[0].Active = 20, 20, true
	g.Bonuses[1].X, g.Bonuses[1].Y, g.Bonuses[1].Active = 40, 20, true

	events := g.Step([]Command{{}, {}, {}})
	if g.Tanks[0].HP != 85 {
		t.Errorf("Tanks[0].HP = %d, want 85", g.Tanks[0].HP)
	}
	if g.Tanks[1].HP != 100 {
		t.Errorf("Tanks[1].HP = %d, want 100 (full HP should not pick up)", g.Tanks[1].HP)
	}
	found := false
	for _, ev := range events {
		if ev.E == "heal" && ev.A == 0 {
			found = true
			if ev.D == nil || *ev.D != 35 {
				t.Errorf("heal event D = %v, want 35", ev.D)
			}
		}
	}
	if !found {
		t.Errorf("events = %+v, want a heal event for tank 0", events)
	}
	if g.Bonuses[0].Active {
		t.Error("Bonuses[0].Active = true, want false right after pickup")
	}
	if g.Bonuses[1].Active != true {
		t.Error("Bonuses[1].Active = false, want true (full-HP tank should not take it)")
	}

	for i := 0; i < 149; i++ {
		g.Step([]Command{{}, {}, {}})
	}
	if g.Bonuses[0].Active {
		t.Error("Bonuses[0].Active = true too early")
	}
	g.Step([]Command{{}, {}, {}})
	if !g.Bonuses[0].Active {
		t.Error("Bonuses[0].Active = false, want true after 150 ticks")
	}
}

func TestZoneShrinksAndDamages(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 1, Y: 1}, {X: 30, Y: 20}})

	g.Tick = 800
	g.Step([]Command{{}, {}})
	approx(t, "Zone.R", g.Zone.R, 37, 1e-9)

	g.Tick = 950
	g.Step([]Command{{}, {}})
	approx(t, "Zone.R", g.Zone.R, 21.5, 1e-9)

	g.Tick = 1100
	g.Step([]Command{{}, {}})
	approx(t, "Zone.R", g.Zone.R, 6, 1e-9)

	g.Tick = 1100
	before := g.Tanks[0].HP
	g.Step([]Command{{}, {}})
	if g.Tanks[0].HP != before-1 {
		t.Errorf("HP = %d, want %d", g.Tanks[0].HP, before-1)
	}

	g.Tick = 1100
	g.Tanks[0].HP = 1
	g.Tanks[0].Alive = true
	events := g.Step([]Command{{}, {}})
	if g.Tanks[0].Alive {
		t.Fatal("tank 0 should have died to the zone")
	}
	found := false
	for _, ev := range events {
		if ev.E == "kill" && ev.A == -1 {
			found = true
			if ev.B == nil || *ev.B != 0 {
				t.Errorf("zone kill event B = %v, want 0", ev.B)
			}
		}
	}
	if !found {
		t.Errorf("events = %+v, want a zone kill event", events)
	}
	if g.Tanks[1].Kills != 0 {
		t.Errorf("Tanks[1].Kills = %d, want 0 (zone kills award no Kills)", g.Tanks[1].Kills)
	}
}

func TestOverWhenOneAlive(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 10, Y: 10}, {X: 20, Y: 20}})
	if g.Over() {
		t.Fatal("Over() = true before anyone died")
	}
	g.Tanks[1].Alive = false
	g.Tanks[1].DeathTick = 5
	if !g.Over() {
		t.Error("Over() = false, want true with one tank left alive")
	}
}

func TestOverAtTickLimit(t *testing.T) {
	g := newTestGame(t, nil, []Point{{X: 10, Y: 10}, {X: 20, Y: 20}})
	g.Tick = g.Rules.Ticks
	if !g.Over() {
		t.Error("Over() = false, want true at the tick limit")
	}
}

func TestDeterministic(t *testing.T) {
	run := func() ([]Frame, []Event) {
		g := New(DefaultRules(), Maps()[1], 3, 42)
		src := rand.New(rand.NewPCG(7, 7))
		var frames []Frame
		var events []Event
		for i := 0; i < 1200; i++ {
			cmds := make([]Command, len(g.Tanks))
			for j := range cmds {
				cmds[j] = Command{
					Move:   src.Float64()*2 - 1,
					Turn:   src.Float64()*2 - 1,
					Turret: src.Float64()*2 - 1,
					Fire:   src.Float64() < 0.3,
				}
			}
			events = append(events, g.Step(cmds)...)
			frames = append(frames, g.Frame())
		}
		return frames, events
	}

	frames1, events1 := run()
	frames2, events2 := run()

	if !reflect.DeepEqual(frames1, frames2) {
		t.Fatal("frames differ between two runs with the same seed and commands")
	}
	if !reflect.DeepEqual(events1, events2) {
		t.Fatal("events differ between two runs with the same seed and commands")
	}
}
