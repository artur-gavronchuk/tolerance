// Package botpkg validates and packs bot archives: what an owner uploads through the dashboard, what
// `arena tanks submit` produces on their machine, or what the platform builds from an agent's diff. It is
// the single gate for what a bot archive may contain. The connector uses PackDir, the server's upload path
// uses Normalize, and the match runner uses Unpack to extract a bot before launching it. Stdlib only.
package botpkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// MaxArchive is the largest a bot archive may be, gzip-compressed, in bytes.
	MaxArchive = 1 << 20
	// MaxUnpacked is the largest a bot archive may be once decompressed, in bytes.
	MaxUnpacked = 4 << 20
	// MaxFiles is the largest number of regular files a bot archive may contain. Directory entries
	// don't count.
	MaxFiles = 200
)

// Manifest is a bot's bot.json, canonicalized: Language is always "python" or "javascript" (an on-disk
// "js" is accepted and normalized to "javascript") and Entry is a clean, relative path that exists in the
// archive as a regular file.
type Manifest struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Entry    string `json:"entry"`
}

// Error is a problem with the bot package itself (not a platform failure) - its Msg is safe to show to
// the bot's author.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

func errf(format string, args ...any) error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}

// entry is one file or directory kept from an archive after junk filtering and, if applicable, wrapper
// stripping. data is nil for directories.
type entry struct {
	name  string
	isDir bool
	data  []byte
}

// errUnpacked is the sentinel countingReader reports once more bytes have been decompressed than
// MaxUnpacked allows; readArchive turns it into an *Error.
var errUnpacked = errors.New("botpkg: unpacked size exceeds limit")

// countingReader wraps a decompressing reader and refuses to hand back more than max bytes in total, no
// matter what any tar header claims about entry sizes - the guard against a decompression bomb is on what
// is actually read out of the gzip stream, not on declared sizes.
type countingReader struct {
	r   io.Reader
	n   int64
	max int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	if c.n >= c.max {
		return 0, errUnpacked
	}
	if remain := c.max - c.n; int64(len(p)) > remain {
		p = p[:remain]
	}
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// isMacJunk reports whether a cleaned, forward-slash archive path is macOS packaging debris:
// __MACOSX/..., any "._*" file, or .DS_Store.
func isMacJunk(clean string) bool {
	if clean == "__MACOSX" || strings.HasPrefix(clean, "__MACOSX/") {
		return true
	}
	base := path.Base(clean)
	return base == ".DS_Store" || strings.HasPrefix(base, "._")
}

// readArchive decompresses and parses a tar.gz, dropping macOS junk, enforcing MaxArchive, MaxUnpacked and
// MaxFiles, and refusing illegal paths and non-file/non-directory entries. It does not look at bot.json at
// all - that is validate's job, once wrapper stripping has run.
func readArchive(archive []byte) ([]entry, error) {
	if len(archive) > MaxArchive {
		return nil, errf("bot archive: compressed size exceeds %d bytes", MaxArchive)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, errf("bot archive: not a valid gzip archive")
	}
	defer gz.Close()

	cr := &countingReader{r: gz, max: MaxUnpacked}
	tr := tar.NewReader(cr)

	var entries []entry
	fileCount := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if errors.Is(err, errUnpacked) {
				return nil, errf("bot archive: unpacked size exceeds %d bytes", MaxUnpacked)
			}
			return nil, errf("bot archive: corrupt tar entry: %v", err)
		}

		clean := path.Clean(strings.TrimSuffix(filepath.ToSlash(h.Name), "/"))
		if clean == "." {
			continue // the archive root itself
		}
		if isMacJunk(clean) {
			continue
		}
		if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, errf("bot archive: illegal path %q", h.Name)
		}

		var isDir bool
		switch h.Typeflag {
		case tar.TypeDir:
			isDir = true
		case tar.TypeReg, tar.TypeRegA:
			isDir = false
		default:
			return nil, errf("bot archive: %q is not a regular file or directory", clean)
		}

		var data []byte
		if !isDir {
			fileCount++
			if fileCount > MaxFiles {
				return nil, errf("bot archive: more than %d files", MaxFiles)
			}
			data, err = io.ReadAll(tr)
			if err != nil {
				if errors.Is(err, errUnpacked) {
					return nil, errf("bot archive: unpacked size exceeds %d bytes", MaxUnpacked)
				}
				return nil, errf("bot archive: corrupt tar entry %q: %v", clean, err)
			}
		}

		entries = append(entries, entry{name: clean, isDir: isDir, data: data})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	return entries, nil
}

// stripWrapper removes a single shared top-level directory (the "mybot/" an archive tool wraps everything
// in) when every remaining entry sits under it, it actually has something nested inside it, and there is
// no bot.json at the archive root already.
func stripWrapper(entries []entry) []entry {
	hasRootBotJSON := false
	top := map[string]struct{}{}
	for _, e := range entries {
		if e.name == "bot.json" && !e.isDir {
			hasRootBotJSON = true
		}
		seg, _, _ := strings.Cut(e.name, "/")
		top[seg] = struct{}{}
	}
	if hasRootBotJSON || len(top) != 1 {
		return entries
	}
	var prefix string
	for d := range top {
		prefix = d
	}
	hasNested := false
	for _, e := range entries {
		if strings.HasPrefix(e.name, prefix+"/") {
			hasNested = true
			break
		}
	}
	if !hasNested {
		return entries
	}

	out := make([]entry, 0, len(entries))
	for _, e := range entries {
		if e.name == prefix {
			continue // the wrapper directory entry itself
		}
		out = append(out, entry{name: strings.TrimPrefix(e.name, prefix+"/"), isDir: e.isDir, data: e.data})
	}
	return out
}

// normalizeLanguage maps every spelling the platform accepts to the canonical value stored in a
// Manifest: "js" is accepted as an alias for "javascript" so bot authors aren't punished for it, but only
// the canonical spelling is ever stored or returned.
func normalizeLanguage(lang string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "python":
		return "python", true
	case "javascript", "js":
		return "javascript", true
	default:
		return "", false
	}
}

func findEntry(entries []entry, name string) (entry, bool) {
	for _, e := range entries {
		if !e.isDir && e.name == name {
			return e, true
		}
	}
	return entry{}, false
}

// manifestFrom reads and validates bot.json out of a (junk-filtered, wrapper-stripped) entry list.
func manifestFrom(entries []entry) (Manifest, error) {
	botJSON, ok := findEntry(entries, "bot.json")
	if !ok {
		return Manifest{}, errf("bot.json: not found")
	}

	var raw Manifest
	if err := json.Unmarshal(botJSON.data, &raw); err != nil {
		return Manifest{}, errf("bot.json: invalid JSON: %v", err)
	}

	name := strings.TrimSpace(raw.Name)
	if name == "" {
		return Manifest{}, errf(`bot.json: "name" must not be empty`)
	}
	if len(name) > 64 {
		return Manifest{}, errf(`bot.json: "name" must be 64 characters or fewer`)
	}

	lang, ok := normalizeLanguage(raw.Language)
	if !ok {
		return Manifest{}, errf("bot.json: unsupported language %q", raw.Language)
	}

	if raw.Entry == "" {
		return Manifest{}, errf(`bot.json: "entry" must not be empty`)
	}
	entryPath := path.Clean(raw.Entry)
	if path.IsAbs(entryPath) || entryPath == ".." || strings.HasPrefix(entryPath, "../") {
		return Manifest{}, errf(`bot.json: "entry" has an illegal path %q`, raw.Entry)
	}
	if _, ok := findEntry(entries, entryPath); !ok {
		return Manifest{}, errf(`bot.json: "entry" file %q not found`, entryPath)
	}

	return Manifest{Name: name, Language: lang, Entry: entryPath}, nil
}

// validate is the shared core of Validate, Normalize and Unpack: parse, drop junk, strip a wrapper
// directory if there is one, and check bot.json. It returns the entries that survived, so callers that
// need to re-pack or extract them don't have to parse the archive a second time.
func validate(archive []byte) (Manifest, []entry, error) {
	entries, err := readArchive(archive)
	if err != nil {
		return Manifest{}, nil, err
	}
	entries = stripWrapper(entries)
	m, err := manifestFrom(entries)
	if err != nil {
		return Manifest{}, nil, err
	}
	return m, entries, nil
}

// Validate checks an uploaded archive and returns its manifest. Accepts tar.gz. Rules: ≤ MaxArchive
// compressed, ≤ MaxUnpacked and ≤ MaxFiles after ignoring macOS junk (__MACOSX/..., any "._*" file,
// .DS_Store); regular files and directories only; no absolute paths or "..". If every remaining entry sits
// under one top-level directory, that directory is stripped. bot.json must exist at the (stripped) root,
// have a name, a supported language and an entry that exists as a regular file and has no "..".
func Validate(archive []byte) (Manifest, error) {
	m, _, err := validate(archive)
	return m, err
}

// packEntries writes files (skipping directory entries, which nothing downstream needs) into a
// deterministic tar.gz: sorted by name, zero mtimes, mode 0644. bot.json's content is replaced with m
// marshaled, so a "js" alias on the way in is "javascript" in what actually gets stored and run.
func packEntries(entries []entry, m Manifest) ([]byte, error) {
	canonical, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("botpkg: marshal manifest: %w", err)
	}

	files := make([]entry, 0, len(entries))
	for _, e := range entries {
		if e.isDir {
			continue
		}
		if e.name == "bot.json" {
			e.data = canonical
		}
		files = append(files, e)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.data))}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Normalize returns Validate's view of the archive re-packed deterministically (junk dropped, wrapper
// directory stripped, a "js" language alias canonicalized to "javascript" in both bot.json and the
// returned Manifest), so what is stored is exactly what runs.
func Normalize(archive []byte) ([]byte, Manifest, error) {
	m, entries, err := validate(archive)
	if err != nil {
		return nil, Manifest{}, err
	}
	out, err := packEntries(entries, m)
	if err != nil {
		return nil, Manifest{}, err
	}
	return out, m, nil
}

// Unpack extracts a normalized archive into dst. It shares validate's walker, so it refuses anything
// Validate would refuse, writes files 0644 and directories 0755, and - because readArchive never produces
// a symlink entry in the first place - never follows or creates one.
func Unpack(archive []byte, dst string) error {
	_, entries, err := validate(archive)
	if err != nil {
		return err
	}
	for _, e := range entries {
		target := filepath.Join(dst, filepath.FromSlash(e.name))
		if e.isDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("botpkg: mkdir %s: %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("botpkg: mkdir %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, e.data, 0o644); err != nil {
			return fmt.Errorf("botpkg: write %s: %w", target, err)
		}
	}
	return nil
}

// isSkippedDir reports whether a directory (by base name) is dropped wholesale by PackDir: a dotfile-style
// directory (".git", ...) or a language-tooling cache directory.
func isSkippedDir(name string) bool {
	return name == "__pycache__" || name == "node_modules" || strings.HasPrefix(name, ".")
}

// isSkippedFile reports whether a file (by base name) is dropped by PackDir: project docs that live
// alongside a bot but aren't part of it, the connector's own log, and dotfiles.
func isSkippedFile(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "GAME.md", "RESULTS.md", "TASK.md", "arena-agent.log":
		return true
	default:
		return false
	}
}

// PackDir packs a bot directory into a deterministic tar.gz (sorted, zero mtimes, mode 0644), skipping
// GAME.md, RESULTS.md, TASK.md, arena-agent.log, dotfiles and dot-directories (.git), __pycache__ and
// node_modules. A symlink anywhere is an *Error. The result is validated with Validate.
func PackDir(dir string) ([]byte, Manifest, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return errf("bot directory: %q is a symlink, not allowed", filepath.ToSlash(rel))
		}
		if d.IsDir() {
			if isSkippedDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isSkippedFile(d.Name()) {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		var pkgErr *Error
		if errors.As(err, &pkgErr) {
			return nil, Manifest{}, pkgErr
		}
		return nil, Manifest{}, fmt.Errorf("botpkg: walk %s: %w", dir, err)
	}
	sort.Strings(files)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, p := range files {
		rel, _ := filepath.Rel(dir, p)
		body, err := os.ReadFile(p)
		if err != nil {
			return nil, Manifest{}, fmt.Errorf("botpkg: read %s: %w", p, err)
		}
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(body))}); err != nil {
			return nil, Manifest{}, err
		}
		if _, err := tw.Write(body); err != nil {
			return nil, Manifest{}, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, Manifest{}, err
	}
	if err := gz.Close(); err != nil {
		return nil, Manifest{}, err
	}

	archive := buf.Bytes()
	m, err := Validate(archive)
	if err != nil {
		return nil, Manifest{}, err
	}
	return archive, m, nil
}
