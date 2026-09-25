package match

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"tolerance/internal/games/tanks"
	"tolerance/internal/platform/sanitize"
)

// Bot outcome statuses.
const (
	StatusOK      = "ok"
	StatusCrashed = "crashed"
	StatusTimeout = "timeout"
	StatusInvalid = "invalid"
)

// Config tunes one match. A zero Config gets sane defaults (see withDefaults).
type Config struct {
	Seed         int64
	Map          string        // "" → tanks.PickMap(Seed)
	Ticks        int           // 0 → rules default
	ReadyTimeout time.Duration // 0 → 5s
	TickTimeout  time.Duration // 0 → 200ms
	FreeTickTime time.Duration // 0 → 20ms
	Budget       time.Duration // 0 → 20s
	MaxNoise     int           // 0 → 1000
	StderrLimit  int           // 0 → 16 KiB
}

func withDefaults(cfg Config) Config {
	if cfg.ReadyTimeout == 0 {
		cfg.ReadyTimeout = 5 * time.Second
	}
	if cfg.TickTimeout == 0 {
		cfg.TickTimeout = 200 * time.Millisecond
	}
	if cfg.FreeTickTime == 0 {
		cfg.FreeTickTime = 20 * time.Millisecond
	}
	if cfg.Budget == 0 {
		cfg.Budget = 20 * time.Second
	}
	if cfg.MaxNoise == 0 {
		cfg.MaxNoise = 1000
	}
	if cfg.StderrLimit == 0 {
		cfg.StderrLimit = 16 * 1024
	}
	return cfg
}

// Player is one match participant: a name (shown to every bot, including itself) and how to launch it.
type Player struct {
	Name string
	Spec Spec
}

// PlayerOutcome is one player's final record of a match.
type PlayerOutcome struct {
	Slot      int
	Place     int
	Kills     int
	Damage    int
	DeathTick *int
	Status    string
	Ready     bool
	Answered  int // ticks answered in time while alive
	Asked     int // ticks sent while alive and not disabled
	Noise     int // stdout lines that were not commands
	Stderr    string
}

// Result is the outcome of one match.
type Result struct {
	Replay  tanks.Replay // Players filled with Slot and Name only; the caller adds bot ids
	Players []PlayerOutcome
}

// botState is Run's bookkeeping for one player's bot across the match.
type botState struct {
	bot       Bot
	status    string
	disabled  bool
	ready     bool
	answered  int
	asked     int
	noise     int
	remaining time.Duration
}

// Run plays one match. It returns an error only when the platform failed (a Launch error); everything a bot
// does wrong is recorded in its outcome. All bots are closed before Run returns.
func Run(ctx context.Context, l Launcher, cfg Config, players []Player) (Result, error) {
	n := len(players)
	if n < 2 || n > 4 {
		return Result{}, fmt.Errorf("match: need 2..4 players, got %d", n)
	}
	cfg = withDefaults(cfg)

	m := tanks.PickMap(cfg.Seed)
	if cfg.Map != "" {
		found, ok := tanks.MapByName(cfg.Map)
		if !ok {
			return Result{}, fmt.Errorf("match: unknown map %q", cfg.Map)
		}
		m = found
	}
	rules := tanks.DefaultRules()
	if cfg.Ticks != 0 {
		rules.Ticks = cfg.Ticks
	}
	g := tanks.New(rules, m, n, cfg.Seed)

	names := make([]string, n)
	for i, p := range players {
		names[i] = p.Name
	}

	// bots is populated one entry per player as each Launch succeeds. The deferred close below is
	// registered before any Launch call so it sees (and closes) whatever prefix of bots got started,
	// on every return path: a Launch error, ctx cancellation, or normal completion.
	bots := make([]*botState, 0, n)
	defer func() {
		for _, b := range bots {
			b.bot.Close()
		}
	}()

	for _, p := range players {
		b, err := l.Launch(ctx, p.Spec)
		if err != nil {
			return Result{}, fmt.Errorf("match: launching %q: %w", p.Name, err)
		}
		bots = append(bots, &botState{bot: b, status: StatusOK, remaining: cfg.Budget})
	}

	var readyWG sync.WaitGroup
	for i := range bots {
		readyWG.Add(1)
		go func(i int) {
			defer readyWG.Done()
			waitReady(ctx, bots[i], tanks.StartFor(g, i, names), cfg.ReadyTimeout)
		}(i)
	}
	readyWG.Wait()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	frames := []tanks.Frame{g.Frame()}
	events := []tanks.Event{}

	for !g.Over() {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}

		tick := g.Tick
		data, _ := json.Marshal(tanks.TickFor(g))

		cmds := make([]tanks.Command, n)
		var tickWG sync.WaitGroup
		for i := range bots {
			b := bots[i]
			if b.disabled || !g.Tanks[i].Alive {
				continue
			}
			tickWG.Add(1)
			go func(i int, b *botState) {
				defer tickWG.Done()
				cmds[i] = collectTick(ctx, b, data, tick, cfg)
			}(i, b)
		}
		tickWG.Wait()
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}

		events = append(events, g.Step(cmds)...)
		frames = append(frames, g.Frame())
	}

	return finish(cfg, g, m, rules, players, bots, frames, events), nil
}

// waitReady sends the start message and waits for a {"type":"ready"} reply, up to timeout. Lines that
// don't parse as a ready object are counted as noise; a closed Lines channel is a crash.
func waitReady(ctx context.Context, b *botState, start tanks.StartMsg, timeout time.Duration) {
	data, _ := json.Marshal(start)
	if err := b.bot.Send(data); err != nil {
		b.status = StatusCrashed
		b.disabled = true
		return
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-b.bot.Lines():
			if !ok {
				b.status = StatusCrashed
				b.disabled = true
				return
			}
			if isReadyLine(line) {
				b.ready = true
				return
			}
			b.noise++
		case <-timer.C:
			b.status = StatusTimeout
			b.disabled = true
			b.ready = false
			return
		}
	}
}

func isReadyLine(line []byte) bool {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return false
	}
	return probe.Type == "ready"
}

// collectTick sends this tick's state to b and waits for its matching command, up to
// min(cfg.TickTimeout, cfg.FreeTickTime+b.remaining). Stray lines count as noise (disabling the bot past
// cfg.MaxNoise); a command for a different tick is discarded, not counted. Time spent beyond
// cfg.FreeTickTime is charged against b.remaining; running out disables the bot as a timeout.
func collectTick(ctx context.Context, b *botState, line []byte, tick int, cfg Config) tanks.Command {
	b.asked++
	start := time.Now()

	if err := b.bot.Send(line); err != nil {
		b.status = StatusCrashed
		b.disabled = true
		return tanks.Command{}
	}

	deadline := cfg.TickTimeout
	if budget := cfg.FreeTickTime + b.remaining; budget < deadline {
		deadline = budget
	}
	if deadline < 0 {
		deadline = 0
	}
	timer := time.NewTimer(deadline)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return tanks.Command{}
		case reply, ok := <-b.bot.Lines():
			if !ok {
				b.status = StatusCrashed
				b.disabled = true
				return tanks.Command{}
			}
			cmd, ok := tanks.ParseCommand(reply)
			if !ok {
				b.noise++
				if b.noise > cfg.MaxNoise {
					b.status = StatusInvalid
					b.disabled = true
					return tanks.Command{}
				}
				continue
			}
			if cmd.Tick != tick {
				continue // a late (or early) answer: discarded, not counted either way
			}
			b.answered++
			chargeBudget(b, time.Since(start), cfg)
			return cmd.Command()
		case <-timer.C:
			chargeBudget(b, time.Since(start), cfg)
			return tanks.Command{}
		}
	}
}

func chargeBudget(b *botState, spent time.Duration, cfg Config) {
	if over := spent - cfg.FreeTickTime; over > 0 {
		b.remaining -= over
	}
	if b.remaining <= 0 {
		b.status = StatusTimeout
		b.disabled = true
	}
}

// finish computes placements, tells every still-enabled bot how the match ended, closes every bot and
// assembles the Result (with sanitized stderr tails).
func finish(cfg Config, g *tanks.Game, m tanks.Map, rules tanks.Rules, players []Player, bots []*botState, frames []tanks.Frame, events []tanks.Event) Result {
	n := len(players)
	placements := g.Placements()

	results := make([]tanks.PlayerResult, n)
	for i, b := range bots {
		t := g.Tanks[i]
		var deathTick *int
		if !t.Alive {
			dt := t.DeathTick
			deathTick = &dt
		}
		results[i] = tanks.PlayerResult{
			Slot:      i,
			Place:     placements[i].Place,
			Kills:     t.Kills,
			Damage:    t.Damage,
			DeathTick: deathTick,
			Status:    b.status,
		}
	}

	for i, b := range bots {
		if b.disabled {
			continue
		}
		end := tanks.EndMsg{Type: "end", Place: placements[i].Place, Players: results}
		data, _ := json.Marshal(end)
		_ = b.bot.Send(data) // errors ignored: the match is over regardless
	}

	for _, b := range bots {
		b.bot.Close()
	}

	replayPlayers := make([]tanks.ReplayPlayer, n)
	outcomes := make([]PlayerOutcome, n)
	for i, b := range bots {
		replayPlayers[i] = tanks.ReplayPlayer{Slot: i, Name: players[i].Name}
		outcomes[i] = PlayerOutcome{
			Slot:      i,
			Place:     results[i].Place,
			Kills:     results[i].Kills,
			Damage:    results[i].Damage,
			DeathTick: results[i].DeathTick,
			Status:    b.status,
			Ready:     b.ready,
			Answered:  b.answered,
			Asked:     b.asked,
			Noise:     b.noise,
			Stderr:    sanitize.CleanLog(b.bot.Stderr(), cfg.StderrLimit),
		}
	}

	walls := make([]tanks.Rect, len(m.Walls))
	copy(walls, m.Walls)

	replay := tanks.Replay{
		Version:  1,
		Engine:   tanks.EngineVersion,
		Seed:     cfg.Seed,
		Map:      m.Name,
		TickRate: rules.TickRate,
		Rules:    rules,
		Walls:    walls,
		Players:  replayPlayers,
		Frames:   frames,
		Events:   events,
		Result:   results,
	}

	return Result{Replay: replay, Players: outcomes}
}
