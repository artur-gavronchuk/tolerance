// Package stacks turns the free-text "made with" of daily submissions into canonical (tool, model) pairs and
// reports which agent stacks actually do well at the daily task. Normalization happens at read time, so rules
// can change without a migration.
package stacks

import (
	"regexp"
	"strings"
)

const other = "Other"

type rule struct {
	name string
	re   *regexp.Regexp
}

func rules(pairs ...[2]string) []rule {
	out := make([]rule, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, rule{name: p[0], re: regexp.MustCompile(p[1])})
	}
	return out
}

// Rules are tried in order on the lower-cased text; the first match wins.
var toolRules = rules(
	[2]string{"Gemini CLI", `gemini[ -]?cli`},
	[2]string{"Claude Code", `claude[ -]?code|claudecode|\bcc\b`},
	[2]string{"Codex CLI", `\bcodex\b`},
	[2]string{"Cursor", `\bcursor\b`},
	[2]string{"Aider", `\baider\b`},
	[2]string{"Cline", `\bcline\b|\broo[ -]?code\b`},
	[2]string{"Copilot", `copilot`},
	[2]string{"Windsurf", `windsurf|\bcascade\b`},
	[2]string{"OpenHands", `open[ -]?hands|opendevin`},
	[2]string{"Custom", `\bcustom\b|own agent|my agent|my own|\bscripts?\b|home[ -]?(made|grown)|self[ -]?(made|written)|\bby hand\b|\bmanual(ly)?\b|\bnone\b|\bno ai\b|\bno agent\b`},
)

var modelRules = rules(
	[2]string{"Opus", `opus`},
	[2]string{"Sonnet", `sonnet`},
	[2]string{"Haiku", `haiku`},
	[2]string{"Fable", `fable`},
	[2]string{"GPT-5", `gpt[ -]?5`},
	[2]string{"GPT-4", `gpt[ -]?4`},
	[2]string{"o-series", `(^|[^a-z0-9])o[134]([ -]?(mini|pro|preview))?($|[^a-z0-9])`},
	[2]string{"Gemini", `gemini`},
	[2]string{"DeepSeek", `deep[ -]?seek`},
	[2]string{"Qwen", `qwen`},
	[2]string{"Grok", `grok`},
	[2]string{"GPT", `\bgpt\b|chatgpt`},
)

func match(rs []rule, s string) string {
	for _, r := range rs {
		if r.re.MatchString(s) {
			return r.name
		}
	}
	return other
}

// Normalize maps a "made with" string to a canonical tool and model; unknown parts are "Other".
func Normalize(madeWith string) (tool, model string) {
	s := strings.ToLower(strings.TrimSpace(madeWith))
	s = strings.NewReplacer("_", " ", "\t", " ").Replace(s)
	if s == "" {
		return other, other
	}
	return match(toolRules, s), match(modelRules, s)
}

// Label is the display name of a stack.
func Label(tool, model string) string {
	switch {
	case tool == other && model == other:
		return other
	case model == other, strings.HasPrefix(tool, model):
		return tool
	case tool == other:
		return model
	}
	return tool + " + " + model
}
