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
