// Package tasks embeds the published task bundles (conditions, dataset and
// check manifest) that competitions are created from. A bundle is
// identified by its directory name, which is also the check suite name.
package tasks

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
)

//go:embed */*.json
var files embed.FS

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// requiredTaskKeys are the keys every task.json must carry.
var requiredTaskKeys = []string{
	"version", "what_to_build", "where_it_runs", "what_to_submit", "hosting", "allowed", "forbidden",
	"travel_rule", "requirements", "extras", "ui_contract", "evaluation",
}

// CheckDef is one automated check from the public manifest.
type CheckDef struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Requirement string `json:"requirement"`
	Weight      int    `json:"weight"`
	Required    bool   `json:"required"`
}

// Bundle is everything a competition needs from its task directory.
type Bundle struct {
	Suite        string
	SuiteVersion string
	Task         json.RawMessage
	Places       json.RawMessage
	Checks       []CheckDef
}

// ValidateChecks enforces that weights sum to 100 and ids are unique.
func ValidateChecks(checks []CheckDef) error {
	if len(checks) == 0 {
		return fmt.Errorf("check manifest is empty")
	}
	seen := map[string]bool{}
	sum := 0
	for _, c := range checks {
		if c.ID == "" || seen[c.ID] {
			return fmt.Errorf("check id %q is empty or duplicated", c.ID)
		}
		seen[c.ID] = true
		sum += c.Weight
	}
	if sum != 100 {
		return fmt.Errorf("check weights sum to %d, want 100", sum)
	}
	return nil
}

// Load reads and validates the bundle stored under slug.
func Load(slug string) (Bundle, error) {
	if !slugRe.MatchString(slug) {
		return Bundle{}, fmt.Errorf("tasks: invalid slug %q", slug)
	}
	read := func(name string) ([]byte, error) {
		raw, err := files.ReadFile(slug + "/" + name)
		if err != nil {
			return nil, fmt.Errorf("tasks: %s/%s: %w", slug, name, err)
		}
		return raw, nil
	}
	task, err := read("task.json")
	if err != nil {
		return Bundle{}, err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(task, &keys); err != nil {
		return Bundle{}, fmt.Errorf("tasks: %s/task.json: %w", slug, err)
	}
	for _, k := range requiredTaskKeys {
		if _, ok := keys[k]; !ok {
			return Bundle{}, fmt.Errorf("tasks: %s/task.json is missing %q", slug, k)
		}
	}
	places, err := read("places.json")
	if err != nil {
		return Bundle{}, err
	}
	if !json.Valid(places) {
		return Bundle{}, fmt.Errorf("tasks: %s/places.json is not valid JSON", slug)
	}
	manifest, err := read("checks.json")
	if err != nil {
		return Bundle{}, err
	}
	var m struct {
		Suite   string     `json:"suite"`
		Version string     `json:"version"`
		Checks  []CheckDef `json:"checks"`
	}
	if err := json.Unmarshal(manifest, &m); err != nil {
		return Bundle{}, fmt.Errorf("tasks: %s/checks.json: %w", slug, err)
	}
	if m.Suite != slug || m.Version == "" {
		return Bundle{}, fmt.Errorf("tasks: %s/checks.json must declare suite %q and a version", slug, slug)
	}
	if err := ValidateChecks(m.Checks); err != nil {
		return Bundle{}, fmt.Errorf("tasks: %s: %w", slug, err)
	}
	return Bundle{Suite: m.Suite, SuiteVersion: m.Version, Task: task, Places: places, Checks: m.Checks}, nil
}
