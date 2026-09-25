package tanks

import "testing"

func TestParseCommand(t *testing.T) {
	cases := []struct {
		name string
		line string
		ok   bool
	}{
		{"valid", `{"tick":3,"move":1,"fire":true}`, true},
		{"not json", `hello`, false},
		{"json array", `[]`, false},
		{"missing tick", `{"move":1}`, false},
		{"tick wrong type", `{"tick":"3"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd, ok := ParseCommand([]byte(c.line))
			if ok != c.ok {
				t.Fatalf("ParseCommand(%q) ok = %v, want %v", c.line, ok, c.ok)
			}
			if c.ok && cmd.Tick != 3 {
				t.Errorf("Tick = %v, want 3", cmd.Tick)
			}
		})
	}
}

func TestTickForHidesInactiveBonuses(t *testing.T) {
	g := New(DefaultRules(), arenaMap(), 2, 1)
	g.Bonuses[0].Active = false
	g.Bonuses[1].Active = true

	msg := TickFor(g)
	if msg.Type != "tick" {
		t.Errorf("Type = %q, want %q", msg.Type, "tick")
	}
	if len(msg.Bonuses) != 3 {
		t.Fatalf("len(Bonuses) = %d, want 3 (only active bonuses)", len(msg.Bonuses))
	}
	for _, b := range msg.Bonuses {
		if b.X == g.Bonuses[0].X && b.Y == g.Bonuses[0].Y {
			t.Errorf("inactive bonus %v leaked into TickFor output", b)
		}
	}
}

func TestStartForCarriesRulesAndWalls(t *testing.T) {
	m := arenaMap()
	g := New(DefaultRules(), m, 2, 1)
	msg := StartFor(g, 1, []string{"alice", "bob"})

	if msg.Type != "start" {
		t.Errorf("Type = %q, want %q", msg.Type, "start")
	}
	if msg.You != 1 {
		t.Errorf("You = %d, want 1", msg.You)
	}
	if msg.Map != m.Name {
		t.Errorf("Map = %q, want %q", msg.Map, m.Name)
	}
	if msg.Rules != g.Rules {
		t.Errorf("Rules = %+v, want %+v", msg.Rules, g.Rules)
	}
	if len(msg.Walls) != len(m.Walls) {
		t.Fatalf("len(Walls) = %d, want %d", len(msg.Walls), len(m.Walls))
	}
	for i, w := range m.Walls {
		if msg.Walls[i] != w {
			t.Errorf("Walls[%d] = %+v, want %+v", i, msg.Walls[i], w)
		}
	}
	if len(msg.Players) != 2 || msg.Players[0].Name != "alice" || msg.Players[1].Name != "bob" {
		t.Errorf("Players = %+v, want alice/bob named by id", msg.Players)
	}
	if msg.Players[0].ID != 0 || msg.Players[1].ID != 1 {
		t.Errorf("Players ids = %+v, want 0,1", msg.Players)
	}
}

func TestCommandClamp(t *testing.T) {
	cmd := CommandMsg{Tick: 1, Move: 5}
	c := cmd.Command()
	if c.Move != 1 {
		t.Errorf("Move = %v, want 1", c.Move)
	}
}
