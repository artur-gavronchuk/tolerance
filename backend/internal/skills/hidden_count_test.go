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

func writeTask(t *testing.T, root, dir, slug string) {
	t.Helper()
	w := func(p, body string) {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w(dir+"/manifest.json", `{"slug": "`+slug+`", "title": "T", "difficulty": 1, "agent_timeout_s": 60, "sandbox_timeout_s": 60, "hidden_tests": 1}`)
	w(dir+"/TASK.md", "Fix it.\n")
	w(dir+"/repo/m.py", "x = 1\n")
	w(dir+"/_hidden/test_hidden_m.py", "def test_hidden_a():\n    pass\n")
}

const pySkill = `{"slug": "python", "title": "Python", "language": "python", "image": "i:1", "run_cmd": "pytest"}`

func TestLoadCatalog_SkipsDotDirsAndRejectsDuplicateSlugs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "python"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "python", "skill.json"), []byte(pySkill), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTask(t, root, "python/a", "py-a")
	if _, tasks, err := skills.LoadCatalog(root); err != nil || len(tasks) != 1 {
		t.Fatalf("dot dir must be skipped: %v %d", err, len(tasks))
	}
	writeTask(t, root, "python/b", "py-a")
	if _, _, err := skills.LoadCatalog(root); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate slug: want error, got %v", err)
	}
}
