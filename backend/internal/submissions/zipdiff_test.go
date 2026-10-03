package submissions

import (
	"archive/zip"
	"bytes"
	"testing"
)

func zipOf(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("x"))
	}
	zw.Close()
	return buf.Bytes()
}

func TestReadZipRefusesUnsafeNames(t *testing.T) {
	for name, names := range map[string][]string{
		"traversal":      {"../evil.py"},
		"absolute":       {"/etc/passwd"},
		"control char":   {"a\x01b.py"},
		"case collision": {"Main.py", "main.py"},
		"file and dir":   {"a", "a/b.py"},
	} {
		if _, err := readZip(zipOf(t, names...)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := readZip(zipOf(t, "src/a.py", "src/b.py")); err != nil {
		t.Errorf("plain zip: %v", err)
	}
}
