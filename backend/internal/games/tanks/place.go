package tanks

import "sort"

// Placement is one tank's final ranking.
type Placement struct {
	Slot  int
	Place int
}

// rankLess reports whether a ranks strictly better than b: alive beats
// dead; among the alive, higher HP then higher damage wins; among the
// dead, a later death tick then higher damage wins.
func rankLess(a, b Tank) bool {
	if a.Alive != b.Alive {
		return a.Alive
	}
	if a.Alive {
		if a.HP != b.HP {
			return a.HP > b.HP
		}
	} else if a.DeathTick != b.DeathTick {
		return a.DeathTick > b.DeathTick
	}
	return a.Damage > b.Damage
}

// rankTie reports whether a and b rank exactly equal.
func rankTie(a, b Tank) bool {
	if a.Alive != b.Alive {
		return false
	}
	if a.Alive {
		return a.HP == b.HP && a.Damage == b.Damage
	}
	return a.DeathTick == b.DeathTick && a.Damage == b.Damage
}

// Placements ranks every tank using competition ranking (1, 1, 3): tied
// tanks share a place, and the next distinct rank skips ahead by the size
// of the tied group. The result is indexed by slot.
func (g *Game) Placements() []Placement {
	n := len(g.Tanks)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return rankLess(g.Tanks[order[i]], g.Tanks[order[j]])
	})

	result := make([]Placement, n)
	place := 1
	for i, slot := range order {
		if i > 0 && !rankTie(g.Tanks[order[i-1]], g.Tanks[slot]) {
			place = i + 1
		}
		result[slot] = Placement{Slot: slot, Place: place}
	}
	return result
}
