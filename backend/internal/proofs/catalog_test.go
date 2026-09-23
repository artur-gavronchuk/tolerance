package proofs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalog_ReadsFixtureAndTarsAreRestorable(t *testing.T) {
	tasks, err := LoadCatalog(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Slug != "go-fix-retry" {
		t.Fatalf("unexpected catalog: %+v", tasks)
	}
	task := tasks[0]
	if task.Image == "" || task.RunCmd == "" || task.AgentTimeoutS == 0 || task.TaskMD == "" || task.RepoSHA256 == "" {
		t.Fatalf("manifest fields missing: %+v", task)
	}
	dst := t.TempDir()
	if err := Untar(task.RepoTar, dst); err != nil {
		t.Fatalf("untar repo: %v", err)
	}
	if err := Untar(task.HiddenTar, dst); err != nil {
		t.Fatalf("untar hidden: %v", err)
	}
	for _, f := range []string{"go.mod", "retry.go", "retry_test.go", "retry_hidden_test.go"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err != nil {
			t.Fatalf("missing %s after untar: %v", f, err)
		}
	}
}

func TestUntar_RejectsPathTraversal(t *testing.T) {
	evil := tarWithEntry(t, "../escape.txt", "x")
	if err := Untar(evil, t.TempDir()); err == nil {
		t.Fatalf("expected traversal to be rejected")
	}
}

func tarWithEntry(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}
