package house

import (
	"testing"

	"tolerance/internal/games/tanks"
)

// play runs strategies directly against a tanks.Game on the "arena" map,
// for up to 1200 ticks or until the game is over, and returns the final
// placements.
func play(t *testing.T, seed int64, names ...string) []tanks.Placement {
	t.Helper()

	m, ok := tanks.MapByName("arena")
	if !ok {
		t.Fatalf("map %q not found", "arena")
	}
	g := tanks.New(tanks.DefaultRules(), m, len(names), seed)

	strategies := make([]Strategy, len(names))
	for i, name := range names {
		s, ok := New(name)
		if !ok {
			t.Fatalf("unknown strategy %q", name)
		}
		strategies[i] = s
	}

	start := tanks.StartFor(g, 0, names)
	for i, s := range strategies {
		you := start
		you.You = i
		s.Start(you)
	}

	for !g.Over() {
		tick := tanks.TickFor(g)
		cmds := make([]tanks.Command, len(strategies))
		for i, s := range strategies {
			reply := s.Decide(tick)
			cmds[i] = reply.Command()
		}
		g.Step(cmds)
	}

	return g.Placements()
}

func placeOf(t *testing.T, placements []tanks.Placement, slot int) int {
	t.Helper()
	for _, p := range placements {
		if p.Slot == slot {
			return p.Place
		}
	}
	t.Fatalf("slot %d not found in placements %+v", slot, placements)
	return 0
}

func TestHunterBeatsIdle(t *testing.T) {
	for seed := int64(1); seed <= 10; seed++ {
		placements := play(t, seed, "hunter", "idle")
		if got := placeOf(t, placements, 0); got != 1 {
			t.Errorf("seed %d: hunter place = %d, want 1 (placements %+v)", seed, got, placements)
		}
	}
}

func TestSniperBeatsIdle(t *testing.T) {
	for seed := int64(1); seed <= 10; seed++ {
		placements := play(t, seed, "sniper", "idle")
		if got := placeOf(t, placements, 0); got != 1 {
			t.Errorf("seed %d: sniper place = %d, want 1 (placements %+v)", seed, got, placements)
		}
	}
}

func TestSniperMostlyBeatsHunter(t *testing.T) {
	wins := 0
	for seed := int64(1); seed <= 20; seed++ {
		placements := play(t, seed, "sniper", "hunter")
		sniper := placeOf(t, placements, 0)
		hunter := placeOf(t, placements, 1)
		if sniper < hunter {
			wins++
		}
	}
	if wins < 11 {
		t.Errorf("sniper beat hunter in %d/20 seeds, want >= 11", wins)
	}
}

func TestDecideEchoesTick(t *testing.T) {
	m, ok := tanks.MapByName("arena")
	if !ok {
		t.Fatalf("map %q not found", "arena")
	}
	g := tanks.New(tanks.DefaultRules(), m, 2, 1)
	for i := 0; i < 5; i++ {
		g.Step([]tanks.Command{{}, {}})
	}

	for _, name := range Names() {
		s, ok := New(name)
		if !ok {
			t.Fatalf("unknown strategy %q", name)
		}
		start := tanks.StartFor(g, 0, []string{"a", "b"})
		s.Start(start)
		tick := tanks.TickFor(g)
		reply := s.Decide(tick)
		if reply.Tick != tick.Tick {
			t.Errorf("%s: reply.Tick = %d, want %d", name, reply.Tick, tick.Tick)
		}
	}
}
