package match

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"tolerance/internal/games/tanks"
)

// ---- test doubles ----

// scriptBot implements Bot over a plain function run in a goroutine, so tests can script a bot's
// behaviour (what it prints, how slow it is, when it crashes) without a real process.
type scriptBot struct {
	in   chan []byte
	out  chan []byte
	stop chan struct{}
	once sync.Once

	mu     sync.Mutex
	closed bool
}

func newScriptBot(fn func(in <-chan []byte, out chan<- []byte)) *scriptBot {
	b := &scriptBot{
		in:   make(chan []byte, 4096),
		out:  make(chan []byte, 4096),
		stop: make(chan struct{}),
	}
	go func() {
		fn(b.in, b.out)
		close(b.out)
	}()
	return b
}

func (b *scriptBot) Send(line []byte) error {
	select {
	case b.in <- line:
		return nil
	case <-b.stop:
		return fmt.Errorf("scriptBot: closed")
	}
}

func (b *scriptBot) Lines() <-chan []byte { return b.out }
func (b *scriptBot) Stderr() string       { return "" }

func (b *scriptBot) Close() error {
	b.once.Do(func() {
		b.mu.Lock()
		b.closed = true
		b.mu.Unlock()
		close(b.stop)
	})
	return nil
}

func (b *scriptBot) Closed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

// scriptLauncher launches a scriptBot keyed by Spec.Dir; any other spec (i.e. a house spec) is left for
// WithHouse to handle.
type scriptLauncher struct {
	scripts map[string]func(in <-chan []byte, out chan<- []byte)
}

func (l *scriptLauncher) Launch(ctx context.Context, s Spec) (Bot, error) {
	fn, ok := l.scripts[s.Dir]
	if !ok {
		return nil, fmt.Errorf("scriptLauncher: no script registered for dir %q", s.Dir)
	}
	return newScriptBot(fn), nil
}

// ---- small protocol helpers for scripts ----

func lineType(line []byte) string {
	var probe struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(line, &probe)
	return probe.Type
}

var readyLine = []byte(`{"type":"ready"}`)

func commandLine(tick int, move float64) []byte {
	data, _ := json.Marshal(tanks.CommandMsg{Tick: tick, Move: move})
	return data
}

func tickOf(line []byte) int {
	var msg tanks.TickMsg
	_ = json.Unmarshal(line, &msg)
	return msg.Tick
}

// respondsPromptly always answers "ready" to start and echoes the requested tick with no movement.
func respondsPromptly(in <-chan []byte, out chan<- []byte) {
	for line := range in {
		switch lineType(line) {
		case "start":
			out <- readyLine
		case "tick":
			out <- commandLine(tickOf(line), 0)
		case "end":
			return
		}
	}
}

// ---- tests ----

func TestHouseMatchCompletes(t *testing.T) {
	launcher := WithHouse(nil)
	players := []Player{
		{Name: "hunter", Spec: Spec{House: "hunter"}},
		{Name: "idle", Spec: Spec{House: "idle"}},
	}
	cfg := Config{Seed: 1, Ticks: 300}

	res, err := Run(context.Background(), launcher, cfg, players)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if res.Players[0].Place != 1 {
		t.Errorf("hunter place = %d, want 1", res.Players[0].Place)
	}
	for i, p := range res.Players {
		if p.Status != StatusOK {
			t.Errorf("player %d status = %q, want %q", i, p.Status, StatusOK)
		}
	}

	frames := res.Replay.Frames
	if len(frames) < 2 {
		t.Fatalf("frames = %d, want at least 2", len(frames))
	}
	last := frames[len(frames)-1]
	if last.T != len(frames)-1 {
		t.Errorf("last frame T = %d, want %d (frame 0 plus one per tick played)", last.T, len(frames)-1)
	}
	if last.T <= 0 || last.T > 300 {
		t.Errorf("ticks played = %d, want in (0, 300]", last.T)
	}
	if res.Replay.Engine != tanks.EngineVersion {
		t.Errorf("engine = %q, want %q", res.Replay.Engine, tanks.EngineVersion)
	}
}

func TestReadyTimeout(t *testing.T) {
	launcher := WithHouse(&scriptLauncher{scripts: map[string]func(in <-chan []byte, out chan<- []byte){
		"silent": func(in <-chan []byte, out chan<- []byte) {
			for range in {
				// never responds to anything, including start
			}
		},
	}})
	players := []Player{
		{Name: "silent", Spec: Spec{Dir: "silent"}},
		{Name: "idle", Spec: Spec{House: "idle"}},
	}
	cfg := Config{Seed: 1, Ticks: 20, ReadyTimeout: 50 * time.Millisecond}

	res, err := Run(context.Background(), launcher, cfg, players)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	silent := res.Players[0]
	if silent.Status != StatusTimeout {
		t.Errorf("status = %q, want %q", silent.Status, StatusTimeout)
	}
	if silent.Ready {
		t.Errorf("ready = true, want false")
	}

	frames := res.Replay.Frames
	last := frames[len(frames)-1]
	if last.T != 20 {
		t.Errorf("match did not play out fully: last tick = %d, want 20", last.T)
	}
}

func TestSlowTickSkipped(t *testing.T) {
	launcher := WithHouse(&scriptLauncher{scripts: map[string]func(in <-chan []byte, out chan<- []byte){
		"slow": func(in <-chan []byte, out chan<- []byte) {
			for line := range in {
				switch lineType(line) {
				case "start":
					out <- readyLine
				case "tick":
					tick := tickOf(line)
					time.Sleep(30 * time.Millisecond)
					out <- commandLine(tick, 0)
				case "end":
					return
				}
			}
		},
	}})
	players := []Player{
		{Name: "slow", Spec: Spec{Dir: "slow"}},
		{Name: "idle", Spec: Spec{House: "idle"}},
	}
	cfg := Config{Seed: 1, Ticks: 20, TickTimeout: 10 * time.Millisecond, Budget: time.Second}

	res, err := Run(context.Background(), launcher, cfg, players)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	slow := res.Players[0]
	if slow.Asked == 0 {
		t.Fatalf("bot was never asked")
	}
	if slow.Answered >= slow.Asked {
		t.Errorf("answered = %d, asked = %d, want answered < asked", slow.Answered, slow.Asked)
	}
}

func TestBudgetExhausted(t *testing.T) {
	launcher := WithHouse(&scriptLauncher{scripts: map[string]func(in <-chan []byte, out chan<- []byte){
		"lagger": func(in <-chan []byte, out chan<- []byte) {
			for line := range in {
				switch lineType(line) {
				case "start":
					out <- readyLine
				case "tick":
					tick := tickOf(line)
					time.Sleep(15 * time.Millisecond)
					out <- commandLine(tick, 0)
				case "end":
					return
				}
			}
		},
	}})
	players := []Player{
		{Name: "lagger", Spec: Spec{Dir: "lagger"}},
		{Name: "idle", Spec: Spec{House: "idle"}},
	}
	cfg := Config{
		Seed:         1,
		Ticks:        30,
		TickTimeout:  50 * time.Millisecond,
		FreeTickTime: time.Millisecond,
		Budget:       100 * time.Millisecond,
	}

	res, err := Run(context.Background(), launcher, cfg, players)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	lagger := res.Players[0]
	if lagger.Status != StatusTimeout {
		t.Errorf("status = %q, want %q", lagger.Status, StatusTimeout)
	}
	// ~15ms/tick against a ~1ms free allowance and a 100ms budget disables the bot well before the
	// 30-tick match ends; the exact tick varies with scheduling, so only check it happened early.
	if lagger.Asked == 0 || lagger.Asked >= 30 {
		t.Errorf("asked = %d, want in [1, 30)", lagger.Asked)
	}
}

func TestNoiseIgnored(t *testing.T) {
	launcher := WithHouse(&scriptLauncher{scripts: map[string]func(in <-chan []byte, out chan<- []byte){
		"chatty": func(in <-chan []byte, out chan<- []byte) {
			for line := range in {
				switch lineType(line) {
				case "start":
					out <- readyLine
				case "tick":
					tick := tickOf(line)
					out <- []byte("thinking...")
					out <- commandLine(tick, 0)
				case "end":
					return
				}
			}
		},
	}})
	players := []Player{
		{Name: "chatty", Spec: Spec{Dir: "chatty"}},
		{Name: "idle", Spec: Spec{House: "idle"}},
	}
	cfg := Config{Seed: 1, Ticks: 100}

	res, err := Run(context.Background(), launcher, cfg, players)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	chatty := res.Players[0]
	if chatty.Asked == 0 {
		t.Fatalf("bot was never asked")
	}
	if chatty.Status != StatusOK {
		t.Errorf("status = %q, want %q", chatty.Status, StatusOK)
	}
	if chatty.Answered != chatty.Asked {
		t.Errorf("answered = %d, asked = %d, want equal", chatty.Answered, chatty.Asked)
	}
	if chatty.Noise != chatty.Asked {
		t.Errorf("noise = %d, asked = %d, want equal", chatty.Noise, chatty.Asked)
	}
}

func TestTooMuchNoise(t *testing.T) {
	launcher := WithHouse(&scriptLauncher{scripts: map[string]func(in <-chan []byte, out chan<- []byte){
		"spammer": func(in <-chan []byte, out chan<- []byte) {
			for line := range in {
				switch lineType(line) {
				case "start":
					out <- readyLine
				case "tick":
					for i := 0; i < 1100; i++ {
						out <- []byte("garbage")
					}
				case "end":
					return
				}
			}
		},
	}})
	players := []Player{
		{Name: "spammer", Spec: Spec{Dir: "spammer"}},
		{Name: "idle", Spec: Spec{House: "idle"}},
	}
	cfg := Config{Seed: 1, Ticks: 20}

	res, err := Run(context.Background(), launcher, cfg, players)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	spammer := res.Players[0]
	if spammer.Status != StatusInvalid {
		t.Errorf("status = %q, want %q", spammer.Status, StatusInvalid)
	}
}

func TestLateAnswerDiscarded(t *testing.T) {
	launcher := WithHouse(&scriptLauncher{scripts: map[string]func(in <-chan []byte, out chan<- []byte){
		"stale": func(in <-chan []byte, out chan<- []byte) {
			for line := range in {
				switch lineType(line) {
				case "start":
					out <- readyLine
				case "tick":
					tick := tickOf(line)
					out <- commandLine(tick-1, 1) // always answers the previous tick, with full throttle
				case "end":
					return
				}
			}
		},
	}})
	players := []Player{
		{Name: "stale", Spec: Spec{Dir: "stale"}},
		{Name: "idle", Spec: Spec{House: "idle"}},
	}
	// Every reply is discarded as stale, so every tick runs out its full deadline; keep that deadline
	// short so the test doesn't spend seconds waiting on replies that will never count.
	cfg := Config{Seed: 1, Ticks: 20, TickTimeout: 20 * time.Millisecond}

	res, err := Run(context.Background(), launcher, cfg, players)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	stale := res.Players[0]
	if stale.Answered != 0 {
		t.Errorf("answered = %d, want 0", stale.Answered)
	}

	frames := res.Replay.Frames
	x0, y0 := frames[0].K[0][0], frames[0].K[0][1]
	for _, f := range frames {
		if f.K[0][0] != x0 || f.K[0][1] != y0 {
			t.Errorf("tank moved: frame %d at (%v,%v), want (%v,%v)", f.T, f.K[0][0], f.K[0][1], x0, y0)
		}
	}
}

func TestCrash(t *testing.T) {
	launcher := WithHouse(&scriptLauncher{scripts: map[string]func(in <-chan []byte, out chan<- []byte){
		"crasher": func(in <-chan []byte, out chan<- []byte) {
			for line := range in {
				if lineType(line) == "start" {
					out <- readyLine
					return // stdout closes right after ready
				}
			}
		},
	}})
	players := []Player{
		{Name: "crasher", Spec: Spec{Dir: "crasher"}},
		{Name: "idle", Spec: Spec{House: "idle"}},
	}
	cfg := Config{Seed: 1, Ticks: 20}

	res, err := Run(context.Background(), launcher, cfg, players)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	crasher := res.Players[0]
	if crasher.Status != StatusCrashed {
		t.Errorf("status = %q, want %q", crasher.Status, StatusCrashed)
	}

	frames := res.Replay.Frames
	x0, y0 := frames[0].K[0][0], frames[0].K[0][1]
	for _, f := range frames {
		if f.K[0][0] != x0 || f.K[0][1] != y0 {
			t.Errorf("tank moved after crash: frame %d at (%v,%v), want (%v,%v)", f.T, f.K[0][0], f.K[0][1], x0, y0)
		}
	}
}

// failingLauncher launches a scriptBot for every call except the given failAt call, which returns a
// platform error instead.
type failingLauncher struct {
	failAt int

	mu    sync.Mutex
	calls int
	bots  []*scriptBot
}

func (l *failingLauncher) Launch(ctx context.Context, s Spec) (Bot, error) {
	l.mu.Lock()
	l.calls++
	call := l.calls
	l.mu.Unlock()

	if call == l.failAt {
		return nil, fmt.Errorf("boom")
	}
	b := newScriptBot(respondsPromptly)
	l.mu.Lock()
	l.bots = append(l.bots, b)
	l.mu.Unlock()
	return b, nil
}

func TestLaunchErrorIsPlatformError(t *testing.T) {
	l := &failingLauncher{failAt: 2}
	players := []Player{
		{Name: "a", Spec: Spec{Dir: "a"}},
		{Name: "b", Spec: Spec{Dir: "b"}},
	}
	cfg := Config{Seed: 1, Ticks: 10}

	_, err := Run(context.Background(), l, cfg, players)
	if err == nil {
		t.Fatalf("Run: want error, got nil")
	}

	if len(l.bots) != 1 {
		t.Fatalf("bots launched = %d, want 1", len(l.bots))
	}
	if !l.bots[0].Closed() {
		t.Errorf("the first bot was not closed")
	}
}

func TestContextCancelled(t *testing.T) {
	l := &failingLauncher{failAt: -1} // never fails
	players := []Player{
		{Name: "a", Spec: Spec{Dir: "a"}},
		{Name: "b", Spec: Spec{Dir: "b"}},
	}
	// A huge tick count so cancellation, not the ticks limit, ends the match.
	cfg := Config{Seed: 1, Ticks: 1_000_000}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	_, err := Run(ctx, l, cfg, players)
	if err != context.Canceled {
		t.Fatalf("Run err = %v, want context.Canceled", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.bots) != 2 {
		t.Fatalf("bots launched = %d, want 2", len(l.bots))
	}
	for i, b := range l.bots {
		if !b.Closed() {
			t.Errorf("bot %d was not closed", i)
		}
	}
}
