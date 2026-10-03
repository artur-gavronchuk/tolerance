package house

// The three planner-driven house bots. They share one engine (planner.go)
// and differ only in which of its abilities are switched on and how sharp
// each is, so the ladder steps are deliberate. Measured against a bot a
// coding agent wrote blind from the starter kit (1v1, ~300 seeds): duelist
// wins about 37% of the time, warden about 68%, ace about 92%.

func baseProfile() profile {
	return profile{
		horizon: 8, twoStage: false, virtual: 0,
		prefRange: 11, rangeBand: 2, strafeW: 1.2, noise: 1.5, hitW: 100,
		healBelow: 50, zoneMargin: 3, aimTol: 1.0,
	}
}

// duelistProfile: leads its shots, dodges shells it can see, strafes at mid
// range, keeps off walls. Never goes for a heal pickup, aims a little
// loosely and moves a little randomly.
func duelistProfile() profile {
	p := baseProfile()
	p.healBelow = 0
	p.aimTol = 0.75
	p.strafeW = 0.4
	p.noise = 3
	return p
}

// wardenProfile: duelist plus the long game. Picks targets by HP and
// exposure, heals when low (unless an enemy who needs it more gets there
// first), drifts to the centre before the zone shrinks, and keeps enemies
// on one side of it in a free-for-all. Fights from a longer range.
func wardenProfile() profile {
	p := baseProfile()
	p.selectTarget = true
	p.healBelow = 50
	p.healEconomy = true
	p.zoneMargin = 4
	p.zonePre = 700
	p.crossfire = 5
	p.cover = 30
	p.prefRange = 13
	p.noise = 3
	p.aimTol = 0.75
	return p
}

// aceProfile: warden that also plans around the shot an enemy is about to
// fire, tries two-step dodges, fights at close range where shells can't be
// dodged, and closes in while the enemy reloads.
func aceProfile() profile {
	p := baseProfile()
	p.twoStage = true
	p.virtual = 1
	p.prefRange = 7
	p.kite = true
	p.selectTarget = true
	p.healBelow = 70
	p.healEconomy = true
	p.zoneMargin = 4
	p.zonePre = 700
	p.crossfire = 5
	p.cover = 30
	return p
}

func newDuelist() Strategy { return newPlanner(duelistProfile()) }
func newWarden() Strategy  { return newPlanner(wardenProfile()) }
func newAce() Strategy     { return newPlanner(aceProfile()) }
