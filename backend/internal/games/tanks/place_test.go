package tanks

import "testing"

func TestPlacementsAliveAboveDead(t *testing.T) {
	g := &Game{Tanks: []Tank{
		{ID: 0, Alive: false, DeathTick: 100, Damage: 50},
		{ID: 1, Alive: true, HP: 10, Damage: 0},
	}}
	places := g.Placements()
	if places[1].Place != 1 {
		t.Errorf("alive tank place = %d, want 1", places[1].Place)
	}
	if places[0].Place != 2 {
		t.Errorf("dead tank place = %d, want 2", places[0].Place)
	}
}

func TestPlacementsDeadByDeathTick(t *testing.T) {
	g := &Game{Tanks: []Tank{
		{ID: 0, Alive: false, DeathTick: 50, Damage: 0},
		{ID: 1, Alive: false, DeathTick: 100, Damage: 0},
	}}
	places := g.Placements()
	if places[1].Place != 1 {
		t.Errorf("later death place = %d, want 1", places[1].Place)
	}
	if places[0].Place != 2 {
		t.Errorf("earlier death place = %d, want 2", places[0].Place)
	}
}

func TestPlacementsTieSharesPlace(t *testing.T) {
	g := &Game{Tanks: []Tank{
		{ID: 0, Alive: true, HP: 80, Damage: 40},
		{ID: 1, Alive: true, HP: 80, Damage: 40},
		{ID: 2, Alive: true, HP: 50, Damage: 10},
	}}
	places := g.Placements()
	if places[0].Place != 1 || places[1].Place != 1 {
		t.Errorf("tied tanks places = %d, %d, want 1, 1", places[0].Place, places[1].Place)
	}
	if places[2].Place != 3 {
		t.Errorf("third tank place = %d, want 3", places[2].Place)
	}
}
