package botpkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// tarNames lists every entry name in a tar.gz archive, in the order it reads them.
func tarNames(t *testing.T, archive []byte) []string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		names = append(names, h.Name)
	}
	return names
}

// archiveEntry is one entry to write into a hand-built test archive.
type archiveEntry struct {
	name string
	typ  byte
	data []byte
}

func buildArchive(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Typeflag: e.typ, Mode: 0o644, Size: int64(len(e.data))}
		if e.typ == tar.TypeDir {
			h.Mode = 0o755
			h.Size = 0
		}
		if e.typ == tar.TypeSymlink {
			h.Linkname = "target"
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatalf("write header %s: %v", e.name, err)
		}
		if e.typ == tar.TypeReg && len(e.data) > 0 {
			if _, err := tw.Write(e.data); err != nil {
				t.Fatalf("write data %s: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPackDirRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bot.json"), []byte(`{"name":"my-bot","language":"python","entry":"bot.py"}`))
	writeFile(t, filepath.Join(dir, "bot.py"), []byte("print('hi')\n"))
	writeFile(t, filepath.Join(dir, "tanks.py"), []byte("# sdk\n"))
	writeFile(t, filepath.Join(dir, "GAME.md"), []byte("# rules\n"))
	writeFile(t, filepath.Join(dir, "RESULTS.md"), []byte("# results\n"))
	writeFile(t, filepath.Join(dir, "TASK.md"), []byte("# task\n"))
	writeFile(t, filepath.Join(dir, "arena-agent.log"), []byte("log\n"))
	writeFile(t, filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"))
	writeFile(t, filepath.Join(dir, "__pycache__", "bot.cpython-312.pyc"), []byte("junk"))
	writeFile(t, filepath.Join(dir, "node_modules", "left-pad", "index.js"), []byte("junk"))

	archive, m, err := PackDir(dir)
	if err != nil {
		t.Fatalf("PackDir: %v", err)
	}
	if m.Name != "my-bot" || m.Language != "python" || m.Entry != "bot.py" {
		t.Fatalf("manifest = %+v", m)
	}

	names := tarNames(t, archive)
	want := map[string]bool{"bot.json": true, "bot.py": true, "tanks.py": true}
	if len(names) != len(want) {
		t.Fatalf("archive entries = %v, want exactly %v", names, want)
	}
	for _, n := range names {
		if !want[n] {
			t.Fatalf("unexpected archive entry %q (full: %v)", n, names)
		}
	}

	dst := t.TempDir()
	if err := Unpack(archive, dst); err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "bot.py"))
	if err != nil || string(got) != "print('hi')\n" {
		t.Fatalf("bot.py = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "GAME.md")); !os.IsNotExist(err) {
		t.Fatalf("GAME.md should not have been unpacked, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git should not have been unpacked, stat err = %v", err)
	}
}

func TestPackDirDeterministic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bot.json"), []byte(`{"name":"my-bot","language":"python","entry":"bot.py"}`))
	writeFile(t, filepath.Join(dir, "bot.py"), []byte("print('hi')\n"))
	writeFile(t, filepath.Join(dir, "tanks.py"), []byte("# sdk\n"))

	a1, _, err := PackDir(dir)
	if err != nil {
		t.Fatalf("PackDir #1: %v", err)
	}
	a2, _, err := PackDir(dir)
	if err != nil {
		t.Fatalf("PackDir #2: %v", err)
	}
	if !bytes.Equal(a1, a2) {
		t.Fatalf("PackDir is not deterministic: %d bytes vs %d bytes", len(a1), len(a2))
	}
}

func TestPackDirRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bot.json"), []byte(`{"name":"my-bot","language":"python","entry":"bot.py"}`))
	writeFile(t, filepath.Join(dir, "bot.py"), []byte("print('hi')\n"))
	if err := os.Symlink(filepath.Join(dir, "bot.py"), filepath.Join(dir, "link.py")); err != nil {
		t.Fatal(err)
	}

	_, _, err := PackDir(dir)
	var pkgErr *Error
	if !errors.As(err, &pkgErr) {
		t.Fatalf("PackDir error = %v (%T), want *Error", err, err)
	}
}

func TestValidateMacOSWrapper(t *testing.T) {
	botJSON := []byte(`{"name":"mybot","language":"python","entry":"bot.py"}`)
	botPy := []byte("print('hi')\n")
	archive := buildArchive(t, []archiveEntry{
		{name: "mybot/bot.json", typ: tar.TypeReg, data: botJSON},
		{name: "mybot/bot.py", typ: tar.TypeReg, data: botPy},
		{name: "__MACOSX/mybot/._bot.py", typ: tar.TypeReg, data: []byte("junk")},
		{name: "mybot/._bot.py", typ: tar.TypeReg, data: []byte("junk")},
		{name: "mybot/.DS_Store", typ: tar.TypeReg, data: []byte("junk")},
	})

	m, err := Validate(archive)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if m.Name != "mybot" || m.Language != "python" || m.Entry != "bot.py" {
		t.Fatalf("manifest = %+v", m)
	}

	normalized, m2, err := Normalize(archive)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if m2 != m {
		t.Fatalf("Normalize manifest = %+v, want %+v", m2, m)
	}
	names := tarNames(t, normalized)
	want := map[string]bool{"bot.json": true, "bot.py": true}
	if len(names) != len(want) {
		t.Fatalf("normalized entries = %v, want exactly %v", names, want)
	}
	for _, n := range names {
		if !want[n] {
			t.Fatalf("unexpected normalized entry %q (full: %v)", n, names)
		}
	}
}

func TestValidateAcceptsJSAlias(t *testing.T) {
	archive := buildArchive(t, []archiveEntry{
		{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"mybot","language":"js","entry":"bot.js"}`)},
		{name: "bot.js", typ: tar.TypeReg, data: []byte("console.log('hi')\n")},
	})

	m, err := Validate(archive)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if m.Language != "javascript" {
		t.Fatalf("Language = %q, want canonical %q", m.Language, "javascript")
	}

	normalized, m2, err := Normalize(archive)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if m2.Language != "javascript" {
		t.Fatalf("Normalize manifest.Language = %q, want %q", m2.Language, "javascript")
	}
	gz, err := gzip.NewReader(bytes.NewReader(normalized))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	found := false
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		if h.Name != "bot.json" {
			continue
		}
		found = true
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte(`"javascript"`)) {
			t.Fatalf("normalized bot.json = %s, want it to say javascript", body)
		}
	}
	if !found {
		t.Fatal("normalized archive has no bot.json")
	}
}

func TestValidateRejects(t *testing.T) {
	files201 := []archiveEntry{
		{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"n","language":"python","entry":"bot.py"}`)},
		{name: "bot.py", typ: tar.TypeReg, data: []byte("pass\n")},
	}
	for i := 0; i < 199; i++ {
		files201 = append(files201, archiveEntry{name: fmt.Sprintf("extra/%03d.txt", i), typ: tar.TypeReg, data: []byte("x")})
	}

	big := bytes.Repeat([]byte{0}, MaxUnpacked+1024)

	cases := []struct {
		name    string
		archive []byte
	}{
		{"not gzip", []byte("this is not a gzip archive at all")},
		{"compressed too big", bytes.Repeat([]byte{0xff}, MaxArchive+1)},
		{"more than 200 files", buildArchive(t, files201)},
		{"unpacked too big", buildArchive(t, []archiveEntry{
			{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"n","language":"python","entry":"bot.py"}`)},
			{name: "bot.py", typ: tar.TypeReg, data: big},
		})},
		{"relative escape", buildArchive(t, []archiveEntry{
			{name: "../x", typ: tar.TypeReg, data: []byte("x")},
		})},
		{"absolute path", buildArchive(t, []archiveEntry{
			{name: "/x", typ: tar.TypeReg, data: []byte("x")},
		})},
		{"symlink entry", buildArchive(t, []archiveEntry{
			{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"n","language":"python","entry":"bot.py"}`)},
			{name: "bot.py", typ: tar.TypeSymlink},
		})},
		{"no bot.json", buildArchive(t, []archiveEntry{
			{name: "bot.py", typ: tar.TypeReg, data: []byte("x")},
		})},
		{"unsupported language", buildArchive(t, []archiveEntry{
			{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"n","language":"rust","entry":"bot.py"}`)},
			{name: "bot.py", typ: tar.TypeReg, data: []byte("x")},
		})},
		{"entry file not found", buildArchive(t, []archiveEntry{
			{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"n","language":"python","entry":"bot.py"}`)},
		})},
		{"entry escapes", buildArchive(t, []archiveEntry{
			{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"n","language":"python","entry":"../x.py"}`)},
		})},
		{"duplicate bot.json", buildArchive(t, []archiveEntry{
			{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"n","language":"python","entry":"bot.py"}`)},
			{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"evil","language":"python","entry":"evil.py"}`)},
			{name: "bot.py", typ: tar.TypeReg, data: []byte("x")},
		})},
		{"file and directory share a path", buildArchive(t, []archiveEntry{
			{name: "stuff", typ: tar.TypeDir},
			{name: "stuff", typ: tar.TypeReg, data: []byte("x")},
		})},
		{"file used as another entry's parent directory", buildArchive(t, []archiveEntry{
			{name: "a", typ: tar.TypeReg, data: []byte("x")},
			{name: "a/b", typ: tar.TypeReg, data: []byte("y")},
		})},
		{"backslash in archive path", buildArchive(t, []archiveEntry{
			{name: `sub\evil.py`, typ: tar.TypeReg, data: []byte("x")},
		})},
		{"backslash in entry field", buildArchive(t, []archiveEntry{
			{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"n","language":"python","entry":"..\\x.py"}`)},
		})},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Validate(c.archive)
			var pkgErr *Error
			if !errors.As(err, &pkgErr) {
				t.Fatalf("Validate(%s) error = %v (%T), want *Error", c.name, err, err)
			}
			if pkgErr.Msg == "" {
				t.Fatalf("Validate(%s): *Error has empty Msg", c.name)
			}
		})
	}
}

func TestValidateEntryNotFoundMessage(t *testing.T) {
	archive := buildArchive(t, []archiveEntry{
		{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"n","language":"python","entry":"bot.py"}`)},
	})
	_, err := Validate(archive)
	var pkgErr *Error
	if !errors.As(err, &pkgErr) {
		t.Fatalf("error = %v, want *Error", err)
	}
	want := `bot.json: "entry" file "bot.py" not found`
	if pkgErr.Msg != want {
		t.Fatalf("Msg = %q, want %q", pkgErr.Msg, want)
	}
}

// TestDuplicateBotJSONRejectedEndToEnd is the reviewer's exact repro for the divergence bug: a raw archive
// with a benign bot.json (entry bot.py) followed by a second, evil bot.json (entry evil.py). Before the
// fix, Validate reported the benign manifest (findEntry returns the first match) while Unpack wrote both
// files, and the second write won because sort.Slice is not stable - so what was checked and what landed
// on disk could disagree. Now the duplicate path is rejected outright, by both Validate and Unpack, so
// there is nothing left for the two to disagree about.
func TestDuplicateBotJSONRejectedEndToEnd(t *testing.T) {
	archive := buildArchive(t, []archiveEntry{
		{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"benign","language":"python","entry":"bot.py"}`)},
		{name: "bot.py", typ: tar.TypeReg, data: []byte("print('benign')\n")},
		{name: "bot.json", typ: tar.TypeReg, data: []byte(`{"name":"evil","language":"python","entry":"evil.py"}`)},
		{name: "evil.py", typ: tar.TypeReg, data: []byte("print('evil')\n")},
	})

	_, err := Validate(archive)
	var validateErr *Error
	if !errors.As(err, &validateErr) {
		t.Fatalf("Validate error = %v (%T), want *Error", err, err)
	}

	err = Unpack(archive, t.TempDir())
	var unpackErr *Error
	if !errors.As(err, &unpackErr) {
		t.Fatalf("Unpack error = %v (%T), want *Error", err, err)
	}
}

func TestValidateNameTooLong(t *testing.T) {
	longName := strings.Repeat("a", 65)
	archive := buildArchive(t, []archiveEntry{
		{name: "bot.json", typ: tar.TypeReg, data: []byte(fmt.Sprintf(`{"name":%q,"language":"python","entry":"bot.py"}`, longName))},
		{name: "bot.py", typ: tar.TypeReg, data: []byte("x")},
	})

	_, err := Validate(archive)
	var pkgErr *Error
	if !errors.As(err, &pkgErr) {
		t.Fatalf("error = %v (%T), want *Error", err, err)
	}
}
