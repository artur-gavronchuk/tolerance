// Package tanks implements the tanks/1 match simulation engine: a
// deterministic, dependency-free physics and rules step for the tanks
// tournament. It has no knowledge of bots, processes or the network —
// callers feed it commands each tick and read back events and state.
package tanks

// EngineVersion identifies the simulation rules and the replay format they
// produce. It is recorded in every replay so a future engine change never
// silently reinterprets an old one.
const EngineVersion = "tanks/1"

// Rules are the tunable parameters of a tanks/1 match.
type Rules struct {
	Width              float64 `json:"width"`
	Height             float64 `json:"height"`
	TickRate           int     `json:"tick_rate"`
	Ticks              int     `json:"ticks"`
	Substeps           int     `json:"substeps"`
	TankRadius         float64 `json:"tank_radius"`
	TankSpeed          float64 `json:"tank_speed"`
	TankReverseSpeed   float64 `json:"tank_reverse_speed"`
	HullTurnRate       float64 `json:"hull_turn_rate"`
	TurretTurnRate     float64 `json:"turret_turn_rate"`
	ReloadTicks        int     `json:"reload_ticks"`
	MuzzleOffset       float64 `json:"muzzle_offset"`
	ShellSpeed         float64 `json:"shell_speed"`
	ShellLifetimeTicks int     `json:"shell_lifetime_ticks"`
	ShellDamage        int     `json:"shell_damage"`
	MaxHP              int     `json:"max_hp"`
	HealAmount         int     `json:"heal_amount"`
	HealRespawnTicks   int     `json:"heal_respawn_ticks"`
	PickupRadius       float64 `json:"pickup_radius"`
	ZoneStartTick      int     `json:"zone_start_tick"`
	ZoneEndTick        int     `json:"zone_end_tick"`
	ZoneStartRadius    float64 `json:"zone_start_radius"`
	ZoneEndRadius      float64 `json:"zone_end_radius"`
	ZoneDamagePerTick  int     `json:"zone_damage_per_tick"`
}

// DefaultRules returns the tanks/1 tournament ruleset: a 60x40 field, 10
// ticks/s for 1200 ticks (2 minutes), 4 physics substeps per tick.
func DefaultRules() Rules {
	return Rules{
		Width:              60,
		Height:             40,
		TickRate:           10,
		Ticks:              1200,
		Substeps:           4,
		TankRadius:         1.0,
		TankSpeed:          5,
		TankReverseSpeed:   3,
		HullTurnRate:       2.5,
		TurretTurnRate:     4,
		ReloadTicks:        10,
		MuzzleOffset:       1.3,
		ShellSpeed:         24,
		ShellLifetimeTicks: 30,
		ShellDamage:        25,
		MaxHP:              100,
		HealAmount:         35,
		HealRespawnTicks:   150,
		PickupRadius:       1.5,
		ZoneStartTick:      800,
		ZoneEndTick:        1100,
		ZoneStartRadius:    37,
		ZoneEndRadius:      6,
		ZoneDamagePerTick:  1,
	}
}
