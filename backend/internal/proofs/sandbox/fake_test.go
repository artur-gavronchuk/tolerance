package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPassAll_ReportsEveryTestFunction(t *testing.T) {
	dir := t.TempDir()
	body := "package x\n\nfunc TestMain(m *testing.M) {}\n\nfunc TestA(t *testing.T) {}\n\nfunc helper() {}\n\nfunc TestB(t *testing.T) {}\n"
	if err := os.WriteFile(filepath.Join(dir, "x_test.go"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package x\n\nfunc TestNotATest() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := PassAll{}.Run(context.Background(), Request{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tests) != 2 || res.Tests[0] != (TestResult{Name: "TestA", Passed: true}) || res.Tests[1] != (TestResult{Name: "TestB", Passed: true}) {
		t.Fatalf("%+v", res.Tests)
	}
}

func TestPassAllPython(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"test_a.py":     "def test_one():\n    pass\n\ndef helper():\n    pass\n",
		"pkg/test_b.py": "def test_two():\n    pass\n",
		"lib.py":        "def test_not_collected():\n    pass\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := PassAll{}.Run(context.Background(), Request{WorkDir: dir, Language: "python"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tests) != 2 || res.Tests[0] != (TestResult{Name: "pkg/test_b.py::test_two", Passed: true}) || res.Tests[1] != (TestResult{Name: "test_a.py::test_one", Passed: true}) {
		t.Fatalf("%+v", res.Tests)
	}
}
