package submissions

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"tolerance/internal/tasks"
)

const (
	maxZipFiles    = 2000
	maxZipUnpacked = 50 << 20
	maxZipFile     = 5 << 20
)

// ignoredDirs are directory names whose contents never belong in a diff: version control, OS junk and
// the caches that test runs and package managers leave next to the code.
var ignoredDirs = map[string]bool{
	".git": true, "__MACOSX": true, "__pycache__": true, ".pytest_cache": true, ".mypy_cache": true,
	".ruff_cache": true, "node_modules": true, ".venv": true, "venv": true, ".idea": true, ".vscode": true,
}

func ignoredFile(base string) bool {
	return base == ".DS_Store" || base == "Thumbs.db" || strings.HasSuffix(base, ".pyc") || strings.HasPrefix(base, "._")
}

// cleanZipPath returns the normalized slash path for a zip entry, ok=false when it must be ignored, and an
// error for entries that try to escape the repository.
func cleanZipPath(name string) (string, bool, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(name, "/") || (len(name) > 1 && name[1] == ':') {
		return "", false, invalid("The zip contains an absolute path: " + name)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return "", false, invalid("The zip contains a path with '..': " + name)
		}
	}
	clean := path.Clean(name)
	if clean == "." || clean == "" {
		return "", false, nil
	}
	segs := strings.Split(clean, "/")
	for _, seg := range segs[:len(segs)-1] {
		if ignoredDirs[seg] {
			return "", false, nil
		}
	}
	if ignoredDirs[segs[len(segs)-1]] || ignoredFile(segs[len(segs)-1]) {
		return "", false, nil
	}
	return clean, true, nil
}

// readZip extracts the files of a zip into memory, refusing symlinks, unsafe paths and oversized archives,
// ignoring junk, and stripping a single top-level directory that holds everything.
func readZip(data []byte) (map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, invalid("The file is not a valid zip archive")
	}
	if len(zr.File) > 20*maxZipFiles {
		return nil, invalid("The zip has too many entries")
	}
	files := map[string][]byte{}
	var total int64
	for _, f := range zr.File {
		if f.Mode()&os.ModeSymlink != 0 {
			return nil, invalid("The zip contains a symbolic link: " + f.Name)
		}
		clean, ok, err := cleanZipPath(f.Name)
		if err != nil {
			return nil, err
		}
		if !ok || f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > maxZipFile {
			return nil, invalid("A file in the zip is larger than 5 MiB: " + f.Name)
		}
		if len(files) >= maxZipFiles {
			return nil, invalid("The zip has more than 2000 files")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, invalid("The zip is damaged")
		}
		body, err := io.ReadAll(io.LimitReader(rc, maxZipFile+1))
		rc.Close()
		if err != nil || len(body) > maxZipFile {
			return nil, invalid("The zip is damaged or a file is too large: " + f.Name)
		}
		total += int64(len(body))
		if total > maxZipUnpacked {
			return nil, invalid("The zip unpacks to more than 50 MiB")
		}
		files[clean] = body
	}
	// One top-level directory holding everything: the person zipped the folder, not its contents.
	var top string
	strip := len(files) > 0
	for p := range files {
		i := strings.Index(p, "/")
		if i < 0 || (top != "" && p[:i] != top) {
			strip = false
			break
		}
		top = p[:i]
	}
	if strip {
		out := make(map[string][]byte, len(files))
		for p, b := range files {
			out[strings.TrimPrefix(p, top+"/")] = b
		}
		files = out
	}
	return files, nil
}

func writeTree(root string, files map[string][]byte) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	for p, body := range files {
		target := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ZipToDiff compares the zip's files with the task's original repo (a tasks tarball) and returns a
// unified diff with a/<path> and b/<path> headers relative to the repo root, ready for `git apply`.
func ZipToDiff(ctx context.Context, repoTar, zipData []byte) (string, error) {
	mod, err := readZip(zipData)
	if err != nil {
		return "", err
	}
	if len(mod) == 0 {
		return "", invalid("The zip has no files")
	}
	orig, err := tasks.ReadTar(repoTar)
	if err != nil {
		return "", err
	}
	// The downloaded zip carries TASK.md next to the repo (tasks.RepoZip); it is not part of the repo,
	// so whatever the upload does with it never reaches the diff.
	if _, inRepo := orig[tasks.TaskFile]; !inRepo {
		delete(mod, tasks.TaskFile)
	}
	tmp, err := os.MkdirTemp("", "diff-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := writeTree(filepath.Join(tmp, "orig"), orig); err != nil {
		return "", err
	}
	if err := writeTree(filepath.Join(tmp, "mod"), mod); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "git", "-c", "core.quotepath=false", "-c", "core.autocrlf=false", "diff", "--no-index", "--no-color",
		"--no-ext-diff", "--no-renames", "--", "orig", "mod")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var ee *exec.ExitError
	if err != nil && !(errors.As(err, &ee) && ee.ExitCode() == 1) { // 1 = "there are differences"
		return "", errors.New("git diff: " + err.Error() + ": " + stderr.String())
	}
	out := normalizeDiffPaths(stdout.String())
	if strings.Contains(out, "\nBinary files ") || strings.HasPrefix(out, "Binary files ") || strings.Contains(out, "GIT binary patch") {
		return "", invalid("The zip changes binary files, which are not supported; remove them and upload again")
	}
	return out, nil
}

// normalizeDiffPaths rewrites the a/orig/<p> and b/mod/<p> file headers `git diff --no-index` prints for
// two directories to a/<p> and b/<p>. Only header lines (from "diff --git" up to the first hunk) are
// touched, so a changed line that happens to start with "--- " is left alone.
func normalizeDiffPaths(diff string) string {
	lines := strings.Split(diff, "\n")
	inHeader := false
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			inHeader = true
			lines[i] = strings.ReplaceAll(strings.ReplaceAll(l, " a/orig/", " a/"), " b/mod/", " b/")
		case strings.HasPrefix(l, "@@"):
			inHeader = false
		case inHeader && strings.HasPrefix(l, "--- a/orig/"):
			lines[i] = "--- a/" + strings.TrimPrefix(l, "--- a/orig/")
		case inHeader && strings.HasPrefix(l, "+++ b/mod/"):
			lines[i] = "+++ b/" + strings.TrimPrefix(l, "+++ b/mod/")
		}
	}
	return strings.Join(lines, "\n")
}

// ReadZip is readZip for other packages (product entries): files by cleaned path, junk dropped, one
// wrapping directory stripped, unsafe archives refused with a 422.
func ReadZip(data []byte) (map[string][]byte, error) { return readZip(data) }
