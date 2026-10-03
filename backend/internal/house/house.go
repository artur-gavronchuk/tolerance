// Package house runs well-known coding agents (Claude Code + Opus, Codex, ...) on every day's task, once, and
// submits their result as a clearly-marked "house" user through the normal submission path, so the verdict is
// computed exactly as for people. The board is never empty, people get a target to beat, and the platform
// gets controlled data on how each agent does.
//
// Configured by ARENA_HOUSE_AGENTS, a path to a JSON file (see backend/house-agents.example.json); unset
// means the feature is off: no users, no jobs.
//
// PROTOTYPE CHOICE: the agent command runs directly on the API host, as the API's OS user, with the host's
// installed CLIs and logins and a scrubbed environment (no ARENA_* secrets). Production must run it in a
// container (no access to the API's filesystem or network, a mounted credential) instead.
package house

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// JobKind is the jobs.kind a house run is queued as.
const JobKind = "run_house_agent"

const (
	defaultTimeoutMin = 45
	maxTimeoutMin     = 180
)

// Agent is one configured house agent.
type Agent struct {
	Handle     string `json:"handle"`      // the house user's handle, lowercase: "opus"
	Name       string `json:"name"`        // display name: "Claude Code · Opus"
	MadeWith   string `json:"made_with"`   // what the submission states: "Claude Code + Opus"
	Command    string `json:"command"`     // run with `sh -c` in the unpacked task; $ARENA_PROMPT is the prompt
	TimeoutMin int    `json:"timeout_min"` // wall-clock limit, default 45
}

var handleRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,29}$`)

// Email is the house user's address: it can never receive mail or be verified by an OAuth provider.
func (a Agent) Email() string { return "house+" + a.Handle + "@tolerance.invalid" }

// UserID is the house user's fixed id, so syncing is idempotent.
func (a Agent) UserID() string { return "user_house_" + a.Handle }

// LoadAgents reads the config file. An empty path means the feature is off.
func LoadAgents(path string) ([]Agent, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ARENA_HOUSE_AGENTS: %w", err)
	}
	var as []Agent
	if err := json.Unmarshal(raw, &as); err != nil {
		return nil, fmt.Errorf("ARENA_HOUSE_AGENTS %s: %w", path, err)
	}
	seen := map[string]bool{}
	for i := range as {
		a := &as[i]
		a.Handle = strings.TrimSpace(a.Handle)
		a.Name, a.MadeWith = strings.TrimSpace(a.Name), strings.TrimSpace(a.MadeWith)
		if !handleRE.MatchString(a.Handle) {
			return nil, fmt.Errorf("ARENA_HOUSE_AGENTS: handle %q must be 2-30 chars of a-z, 0-9, _ or -", a.Handle)
		}
		if seen[a.Handle] {
			return nil, fmt.Errorf("ARENA_HOUSE_AGENTS: duplicate handle %q", a.Handle)
		}
		seen[a.Handle] = true
		if strings.TrimSpace(a.Command) == "" {
			return nil, fmt.Errorf("ARENA_HOUSE_AGENTS: agent %q has no command", a.Handle)
		}
		if a.Name == "" {
			a.Name = a.Handle
		}
		if a.MadeWith == "" {
			a.MadeWith = a.Name
		}
		if a.TimeoutMin <= 0 {
			a.TimeoutMin = defaultTimeoutMin
		}
		if a.TimeoutMin > maxTimeoutMin {
			return nil, errors.New("ARENA_HOUSE_AGENTS: timeout_min is at most 180")
		}
	}
	return as, nil
}

// Prompt is what the agent is told. It matches the prompts the site shows people (AGENT_PROMPT and
// OPTIMIZE_PROMPT in frontend/components/daily/daily-view.tsx): keep them in step.
func Prompt(kind string, direction *string) string {
	if kind == "optimize" {
		verb := "maximize"
		if direction != nil && *direction == "min" {
			verb = "minimize"
		}
		return "Here is a repository and a task description in TASK.md. Improve solve.py to " + verb + " the score described in TASK.md.\n" +
			"Test locally with tools/score.py on the instances in examples/. Keep the output valid and within the time limit per case. Use only the standard library.\n" +
			"Work on your own and do not ask questions; stop when you are done."
	}
	return "Here is a repository and a task description in TASK.md. Read TASK.md and fix the issue it describes.\n" +
		"Do not modify or delete existing tests, and do not add test files. Keep the change minimal and in the style of the surrounding code.\n" +
		"When you are done, the repository should build and the tests should pass.\n" +
		"Work on your own and do not ask questions; stop when you are done."
}
