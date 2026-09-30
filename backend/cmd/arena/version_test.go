package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func cfgWith(model, harness, command string, files ...string) config {
	var c config
	c.URL = "https://a.example"
	c.Agent.Command = command
	c.Agent.Model = model
	c.Agent.Harness = harness
	c.Agent.FingerprintFiles = files
	return c
}

func TestConfigDigest(t *testing.T) {
	fs := map[string][]byte{"/home/u/.claude/CLAUDE.md": []byte("be careful")}
	read := func(p string) ([]byte, error) {
		b, ok := fs[p]
		if !ok {
			return nil, errors.New("missing")
		}
		return b, nil
	}
	base := cfgWith("claude-opus-5-5", "claude-code", "claude -p x", "/home/u/.claude/CLAUDE.md")
	d1, err := configDigest(base, read)
	if err != nil || len(d1) != 64 {
		t.Fatalf("digest: %v %q", err, d1)
	}
	d2, _ := configDigest(base, read)
	if d1 != d2 {
		t.Fatalf("digest must be deterministic")
	}
	other := base
	other.URL = "https://other.example"
	if d3, _ := configDigest(other, read); d3 != d1 {
		t.Fatalf("url must not be part of the digest")
	}
	model := cfgWith("claude-sonnet-5", "claude-code", "claude -p x", "/home/u/.claude/CLAUDE.md")
	if d4, _ := configDigest(model, read); d4 == d1 {
		t.Fatalf("model change must change the digest")
	}
	fs["/home/u/.claude/CLAUDE.md"] = []byte("be bold")
	if d5, _ := configDigest(base, read); d5 == d1 {
		t.Fatalf("fingerprint file change must change the digest")
	}
	missing := cfgWith("m", "h", "c", "/nope")
	_, err = configDigest(missing, read)
	if err == nil || !strings.Contains(err.Error(), "/nope") || !strings.Contains(err.Error(), "fingerprint_files") {
		t.Fatalf("missing fingerprint file must be a clear error, got %v", err)
	}
	none := cfgWith("m", "h", "c")
	if d, err := configDigest(none, read); err != nil || len(d) != 64 {
		t.Fatalf("no fingerprint files is fine: %v", err)
	}
}

// R3: the config `arena init` writes must work on a machine that has none of
// the example files, so fingerprint_files ships commented out.
func TestDefaultConfigHasNoFingerprintFiles(t *testing.T) {
	var c config
	if err := yaml.Unmarshal([]byte(fmt.Sprintf(defaultConfig, "https://a.example")), &c); err != nil {
		t.Fatal(err)
	}
	if c.Agent.Model == "" || c.Agent.Harness == "" {
		t.Fatalf("default config must set model and harness: %+v", c.Agent)
	}
	if len(c.Agent.FingerprintFiles) != 0 {
		t.Fatalf("default config must not activate fingerprint files: %v", c.Agent.FingerprintFiles)
	}
	if !strings.Contains(defaultConfig, "# fingerprint_files:") || !strings.Contains(defaultConfig, "~/.claude/CLAUDE.md") {
		t.Fatalf("default config must keep the example in a comment")
	}
	d, err := configDigest(c, func(string) ([]byte, error) { return nil, os.ErrNotExist })
	if err != nil || len(d) != 64 {
		t.Fatalf("default config must yield a valid digest: %v %q", err, d)
	}
}

func TestFormatStatus_VersionAndSkills(t *testing.T) {
	var st statusResp
	if err := json.Unmarshal([]byte(`{"agent": {"name": "fixer", "stage": "operational",
		"version": {"number": 2, "model": "claude-sonnet-5"},
		"skills": [
			{"skill_slug": "go", "rating": 2014, "uncertainty": 350, "tier": "verified", "version_number": 2, "on_current_version": true},
			{"skill_slug": "python", "rating": 1700, "uncertainty": 350, "tier": "none", "version_number": 1, "on_current_version": false}]},
		"last_proof": null}`), &st); err != nil {
		t.Fatal(err)
	}
	want := "fixer: operational · v2 · model claude-sonnet-5\nlast proof: none yet\ngo: 2014 ± 350 · verified\npython: 1700 ± 350 · on v1, not proven on v2\n"
	if got := formatStatus(st, time.Now()); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}
