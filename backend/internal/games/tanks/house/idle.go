package house

import "tolerance/internal/games/tanks"

// idleStrategy never moves or fires. It exists only as a baseline for
// beats_idle and for tests: a tank that does nothing should lose to
// anything that tries.
type idleStrategy struct{}

func (s *idleStrategy) Start(tanks.StartMsg) {}

func (s *idleStrategy) Decide(t tanks.TickMsg) tanks.CommandMsg {
	return tanks.CommandMsg{Tick: t.Tick}
}
