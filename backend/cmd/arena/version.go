package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// configDigest identifies the agent version: the agent block of the config
// (never the url) plus the hash of every fingerprint file. Only hashes
// leave the machine. A listed file that cannot be read is an error.
func configDigest(cfg config, read func(string) ([]byte, error)) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "model=%s\nharness=%s\ncommand=%s\n", cfg.Agent.Model, cfg.Agent.Harness, cfg.Agent.Command)
	for _, f := range cfg.Agent.FingerprintFiles {
		content, err := read(expandHome(f))
		if err != nil {
			return "", fmt.Errorf("fingerprint file %s: %w (remove it from fingerprint_files or create it)", f, err)
		}
		sum := sha256.Sum256(content)
		fmt.Fprintf(&b, "file %s %s\n", f, hex.EncodeToString(sum[:]))
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}
