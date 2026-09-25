// Package match runs one tanks match: it starts every player's bot (in
// process for house strategies, or through a Launcher for real bot code),
// speaks the tanks protocol (internal/games/tanks) to each of them under a
// budget, and steps the tanks engine to produce a Result and a replay.
package match

import (
	"context"
	"fmt"
	"strings"
)

// Bot is one running bot. Send writes one line (a newline is appended). Lines yields every line the bot
// prints to stdout and is closed when stdout ends. Stderr returns the capped tail of stderr so far.
// Close kills the bot (and anything it started) and waits; it is safe to call more than once.
type Bot interface {
	Send(line []byte) error
	Lines() <-chan []byte
	Stderr() string
	Close() error
}

// Spec says what to launch. House != "" selects an in-process house bot; otherwise Dir holds an unpacked bot.
type Spec struct {
	House    string
	Dir      string
	Language string
	Entry    string
}

// Launcher starts bots. An error means the platform could not start it (docker down, interpreter missing);
// a bot whose own code fails to start is a Bot whose Lines closes immediately.
type Launcher interface {
	Launch(ctx context.Context, s Spec) (Bot, error)
}

// Command returns the argv for a language: python → ["python3", "-u", entry], javascript → ["node", entry].
func Command(language, entry string) ([]string, error) {
	switch strings.ToLower(language) {
	case "python":
		return []string{"python3", "-u", entry}, nil
	case "javascript", "js":
		return []string{"node", entry}, nil
	default:
		return nil, fmt.Errorf("match: unknown bot language %q", language)
	}
}
