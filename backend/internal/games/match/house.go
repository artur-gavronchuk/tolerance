package match

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"tolerance/internal/games/tanks"
	"tolerance/internal/games/tanks/house"
)

// WithHouse serves House specs in process and passes everything else to next (next may be nil in tests
// that only use house bots; a non-house spec then fails with an error).
func WithHouse(next Launcher) Launcher {
	return &houseLauncher{next: next}
}

type houseLauncher struct {
	next Launcher
}

func (l *houseLauncher) Launch(ctx context.Context, s Spec) (Bot, error) {
	if s.House == "" {
		if l.next == nil {
			return nil, fmt.Errorf("match: spec has no house strategy and no launcher configured")
		}
		return l.next.Launch(ctx, s)
	}
	strat, ok := house.New(s.House)
	if !ok {
		return nil, fmt.Errorf("match: unknown house strategy %q", s.House)
	}
	return newHouseBot(strat), nil
}

// houseBot serves a house.Strategy in a goroutine, speaking the same JSON-line protocol a bot process
// would (start/tick/end in, ready/command out), so Run treats it exactly like any other Bot.
type houseBot struct {
	in   chan []byte
	out  chan []byte
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func newHouseBot(strat house.Strategy) *houseBot {
	b := &houseBot{
		in:   make(chan []byte, 8),
		out:  make(chan []byte, 8),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go b.run(strat)
	return b
}

func (b *houseBot) run(strat house.Strategy) {
	defer close(b.done)
	defer close(b.out)
	for {
		select {
		case line, ok := <-b.in:
			if !ok {
				return
			}
			if b.handle(strat, line) {
				return
			}
		case <-b.stop:
			return
		}
	}
}

// handle processes one incoming line and reports whether the bot should stop (an "end" message).
func (b *houseBot) handle(strat house.Strategy, line []byte) (stop bool) {
	typ, ok := messageType(line)
	if !ok {
		return false
	}
	switch typ {
	case "start":
		var msg tanks.StartMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			return false
		}
		strat.Start(msg)
		reply, _ := json.Marshal(struct {
			Type string `json:"type"`
		}{Type: "ready"})
		b.emit(reply)
	case "tick":
		var msg tanks.TickMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			return false
		}
		reply, _ := json.Marshal(strat.Decide(msg))
		b.emit(reply)
	case "end":
		return true
	}
	return false
}

func (b *houseBot) emit(line []byte) {
	select {
	case b.out <- line:
	case <-b.stop:
	}
}

func (b *houseBot) Send(line []byte) error {
	select {
	case b.in <- line:
		return nil
	case <-b.done:
		return fmt.Errorf("match: house bot has stopped")
	}
}

func (b *houseBot) Lines() <-chan []byte { return b.out }

func (b *houseBot) Stderr() string { return "" }

func (b *houseBot) Close() error {
	b.once.Do(func() { close(b.stop) })
	<-b.done
	return nil
}
