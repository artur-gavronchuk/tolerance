// Package house implements the tanks tournament's house bots: strategies
// that run in-process (no subprocess, no protocol serialization) so the
// ladder and the arena UI always have something to show, even before any
// owner has a bot in the tournament.
package house

import "tolerance/internal/games/tanks"

// Strategy decides one tank's move each tick, playing against the same
// tanks.StartMsg/tanks.TickMsg views a real bot process would see.
type Strategy interface {
	Start(m tanks.StartMsg)
	Decide(t tanks.TickMsg) tanks.CommandMsg // Tick in the reply equals t.Tick
}

// Names lists every house strategy, in a fixed order.
func Names() []string {
	return []string{"hunter", "idle", "sniper"}
}

// Ladder lists the house strategies that play in the tournament ladder
// (alongside owner bots) with a "house" mark; idle exists only to give the
// other strategies and the qualification check something trivial to beat.
var Ladder = []string{"hunter", "sniper"}

// New looks up a house strategy by name.
func New(name string) (Strategy, bool) {
	switch name {
	case "idle":
		return &idleStrategy{}, true
	case "hunter":
		return &hunterStrategy{}, true
	case "sniper":
		return &sniperStrategy{}, true
	default:
		return nil, false
	}
}
