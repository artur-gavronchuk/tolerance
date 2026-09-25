package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

type config struct {
	URL   string `yaml:"url"`
	Agent struct {
		Command string `yaml:"command"`
	} `yaml:"agent"`
}

func arenaDir() (string, error) {
	if d := os.Getenv("ARENA_HOME"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".arena"), nil
}

func loadConfig() (config, error) {
	dir, err := arenaDir()
	if err != nil {
		return config{}, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return config{}, fmt.Errorf("read %s/config.yaml (run `arena init`): %w", dir, err)
	}
	var c config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return config{}, err
	}
	if v := os.Getenv("ARENA_URL"); v != "" {
		c.URL = v
	}
	c.URL = strings.TrimRight(c.URL, "/")
	if c.URL == "" || c.Agent.Command == "" {
		return config{}, errors.New("config.yaml needs url and agent.command")
	}
	return c, nil
}

func loadKey() (string, error) {
	dir, err := arenaDir()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "key"))
	if err != nil {
		return "", fmt.Errorf("read %s/key (run `arena login`): %w", dir, err)
	}
	return strings.TrimSpace(string(raw)), nil
}

const defaultConfig = `# tolerance connector configuration.
url: %s
agent:
  # Any shell command. It runs in the root of the task repository; TASK.md
  # describes the task. Everything the command prints stays on this machine
  # except a redacted 32 KiB tail sent with the result.
  command: claude -p "$(cat TASK.md)" --dangerously-skip-permissions
`
