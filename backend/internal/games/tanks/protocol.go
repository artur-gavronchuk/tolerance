package tanks

import "encoding/json"

// PlayerInfo names one slot in the start message's player roster.
type PlayerInfo struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// StartMsg is sent once, before the first tick, to every bot process.
type StartMsg struct {
	Type    string       `json:"type"` // "start"
	You     int          `json:"you"`
	Map     string       `json:"map"`
	Rules   Rules        `json:"rules"`
	Walls   []Rect       `json:"walls"`
	Players []PlayerInfo `json:"players"`
}

// TankView is one tank's state as seen by a bot.
type TankView struct {
	ID     int     `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Hull   float64 `json:"hull"`
	Turret float64 `json:"turret"`
	HP     int     `json:"hp"`
	Reload int     `json:"reload"`
	Alive  bool    `json:"alive"`
}

// ShellView is one shell's state as seen by a bot.
type ShellView struct {
	ID    int     `json:"id"`
	Owner int     `json:"owner"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	VX    float64 `json:"vx"`
	VY    float64 `json:"vy"`
}

// BonusView is one active bonus's position as seen by a bot.
type BonusView struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// TickMsg is sent every tick, to every living bot process, with the full
// game state (there is no fog of war).
type TickMsg struct {
	Type    string      `json:"type"` // "tick"
	Tick    int         `json:"tick"`
	Tanks   []TankView  `json:"tanks"`
	Shells  []ShellView `json:"shells"`
	Bonuses []BonusView `json:"bonuses"` // active bonuses only
	Zone    Zone        `json:"zone"`
}

// EndMsg is sent once, after the match is over, to every bot process.
type EndMsg struct {
	Type    string         `json:"type"` // "end"
	Place   int            `json:"place"`
	Players []PlayerResult `json:"players"`
}

// CommandMsg is one tick's reply from a bot process.
type CommandMsg struct {
	Tick   int     `json:"tick"`
	Move   float64 `json:"move"`
	Turn   float64 `json:"turn"`
	Turret float64 `json:"turret"`
	Fire   bool    `json:"fire"`
}

// StartFor builds the start message for slot you, naming every player from
// names (indexed by slot).
func StartFor(g *Game, you int, names []string) StartMsg {
	players := make([]PlayerInfo, len(names))
	for i, name := range names {
		players[i] = PlayerInfo{ID: i, Name: name}
	}
	walls := make([]Rect, len(g.Map.Walls))
	copy(walls, g.Map.Walls)
	return StartMsg{
		Type:    "start",
		You:     you,
		Map:     g.Map.Name,
		Rules:   g.Rules,
		Walls:   walls,
		Players: players,
	}
}

// TickFor builds the tick message for the game's current state. Inactive
// bonuses are left out — a bot never learns about a pickup on cooldown.
func TickFor(g *Game) TickMsg {
	tanks := make([]TankView, len(g.Tanks))
	for i, t := range g.Tanks {
		tanks[i] = TankView{
			ID:     t.ID,
			X:      t.X,
			Y:      t.Y,
			Hull:   t.Hull,
			Turret: t.Turret,
			HP:     t.HP,
			Reload: t.Reload,
			Alive:  t.Alive,
		}
	}

	shells := make([]ShellView, len(g.Shells))
	for i, s := range g.Shells {
		shells[i] = ShellView{ID: s.ID, Owner: s.Owner, X: s.X, Y: s.Y, VX: s.VX, VY: s.VY}
	}

	bonuses := []BonusView{}
	for _, b := range g.Bonuses {
		if b.Active {
			bonuses = append(bonuses, BonusView{X: b.X, Y: b.Y})
		}
	}

	return TickMsg{
		Type:    "tick",
		Tick:    g.Tick,
		Tanks:   tanks,
		Shells:  shells,
		Bonuses: bonuses,
		Zone:    g.Zone,
	}
}

// Command converts a bot's reply into an engine command, clamping Move,
// Turn and Turret to [-1, 1] the same way Game.Step would.
func (c CommandMsg) Command() Command {
	return Command{
		Move:   clamp1(c.Move),
		Turn:   clamp1(c.Turn),
		Turret: clamp1(c.Turret),
		Fire:   c.Fire,
	}
}

// ParseCommand parses one line of a bot's stdout as a CommandMsg. ok is
// false unless line is a JSON object with a numeric "tick" field — anything
// else (plain text, a JSON array, a missing or non-numeric tick) is the
// "stray stdout line" case the protocol says to ignore and count, not a
// command.
func ParseCommand(line []byte) (cmd CommandMsg, ok bool) {
	var probe map[string]interface{}
	if err := json.Unmarshal(line, &probe); err != nil {
		return CommandMsg{}, false
	}
	if _, isNumber := probe["tick"].(float64); !isNumber {
		return CommandMsg{}, false
	}

	if err := json.Unmarshal(line, &cmd); err != nil {
		return CommandMsg{}, false
	}
	return cmd, true
}
