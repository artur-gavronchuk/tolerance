package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tolerance/internal/skills"
)

func TestLoadCatalog_HiddenCountMustMatchManifest(t *testing.T) {
	root := t.TempDir()
	write := func(p, body string) {
		t.Helper()
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("python/skill.json", `{"slug": "python", "title": "Python", "language": "python", "image": "arena-skill-python:1", "run_cmd": "python -m pytest -q -rA -p no:cacheprovider"}`)
	write("python/Dockerfile", "FROM python:3.12-alpine\n")
	write("python/t1/manifest.json", `{"slug": "py-t1", "title": "T1", "difficulty": 1, "agent_timeout_s": 60, "sandbox_timeout_s": 60, "hidden_tests": 3}`)
	write("python/t1/TASK.md", "Fix it.\n")
	write("python/t1/repo/m.py", "x = 1\n")
	write("python/t1/_hidden/test_hidden_m.py", "def test_hidden_a():\n    pass\n\ndef test_hidden_b():\n    pass\n")
	if _, _, err := skills.LoadCatalog(root); err == nil || !strings.Contains(err.Error(), "hidden") {
		t.Fatalf("manifest says 3, files have 2: want an error, got %v", err)
	}
}
