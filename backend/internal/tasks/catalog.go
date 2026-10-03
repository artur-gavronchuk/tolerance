// Package tasks is the catalog of coding tasks: loaded from fixtures on disk by cmd/migrate, stored in the
// tasks table, and served to people as a repo zip. A task is a small repository with a bug or a missing
// feature, a TASK.md, and hidden tests the platform runs over the result.
package tasks

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
)

// Task is one catalog entry as loaded from disk.
type Task struct {
	Slug            string
	Title           string
	Language        string
	Difficulty      int
	Image           string
	RunCmd          string
	SandboxTimeoutS int
	HiddenTests     int
	TaskMD          string
	RepoTar         []byte
	HiddenTar       []byte
}

type language struct {
	Slug     string `json:"slug"`
	Language string `json:"language"`
	Image    string `json:"image"`
	RunCmd   string `json:"run_cmd"`
}

type manifest struct {
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	Language        string `json:"language"`
	Difficulty      int    `json:"difficulty"`
	Image           string `json:"image"`
	RunCmd          string `json:"run_cmd"`
	SandboxTimeoutS int    `json:"sandbox_timeout_s"`
	HiddenTests     int    `json:"hidden_tests"`
}

// LoadFlat reads a directory of task directories that each carry their own image and run command in
// manifest.json (fixtures/proofs).
func LoadFlat(dir string) ([]Task, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("tasks: read %s: %w", dir, err)
	}
	var out []Task
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		t, err := loadTask(filepath.Join(dir, e.Name()), language{})
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// LoadByLanguage reads fixtures/skills: <lang>/skill.json (language, image, run command) and every task
// directory beside it (a directory with a manifest.json).
func LoadByLanguage(dir string) ([]Task, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("tasks: read %s: %w", dir, err)
	}
	var out []Task
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		ldir := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(filepath.Join(ldir, "skill.json"))
		if err != nil {
			return nil, fmt.Errorf("tasks: %s: %w", ldir, err)
		}
		var l language
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("tasks: %s/skill.json: %w", ldir, err)
		}
		if l.Image == "" || l.RunCmd == "" || l.Language == "" {
			return nil, fmt.Errorf("tasks: %s/skill.json: language, image, run_cmd required", ldir)
		}
		sub, err := os.ReadDir(ldir)
		if err != nil {
			return nil, err
		}
		for _, te := range sub {
			tdir := filepath.Join(ldir, te.Name())
			if !te.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(tdir, "manifest.json")); err != nil {
				continue
			}
			t, err := loadTask(tdir, l)
			if err != nil {
				return nil, err
			}
			out = append(out, t)
		}
	}
	return out, nil
}

// Merge sorts tasks by slug and refuses duplicates.
func Merge(lists ...[]Task) ([]Task, error) {
	var all []Task
	seen := map[string]bool{}
	for _, l := range lists {
		for _, t := range l {
			if seen[t.Slug] {
				return nil, fmt.Errorf("tasks: duplicate slug %q", t.Slug)
			}
			seen[t.Slug] = true
			all = append(all, t)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Slug < all[j].Slug })
	return all, nil
}

func loadTask(dir string, l language) (Task, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Task{}, fmt.Errorf("tasks: %s: %w", dir, err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Task{}, fmt.Errorf("tasks: %s/manifest.json: %w", dir, err)
	}
	if m.Difficulty == 0 {
		m.Difficulty = 1
	}
	if m.Language == "" {
		m.Language = l.Language
	}
	if m.Image == "" {
		m.Image = l.Image
	}
	if m.RunCmd == "" {
		m.RunCmd = l.RunCmd
	}
	if m.Slug == "" || m.Language == "" || m.Image == "" || m.RunCmd == "" || m.SandboxTimeoutS <= 0 || m.HiddenTests <= 0 || m.Difficulty < 1 || m.Difficulty > 3 {
		return Task{}, fmt.Errorf("tasks: %s/manifest.json: slug, language, image, run_cmd, sandbox_timeout_s, hidden_tests and difficulty 1-3 are required", dir)
	}
	md, err := os.ReadFile(filepath.Join(dir, "TASK.md"))
	if err != nil {
		return Task{}, fmt.Errorf("tasks: %s: %w", dir, err)
	}
	repoTar, err := TarDir(filepath.Join(dir, "repo"))
	if err != nil {
		return Task{}, err
	}
	hiddenTar, err := TarDir(filepath.Join(dir, "_hidden"))
	if err != nil {
		return Task{}, err
	}
	// The score divides by hidden_tests and counts hidden tests by name, so the manifest and the files
	// must agree on how many there are.
	names, err := HiddenTestNames(m.Language, hiddenTar)
	if err != nil {
		return Task{}, err
	}
	if len(names) != m.HiddenTests {
		return Task{}, fmt.Errorf("tasks: %s: manifest says %d hidden tests, _hidden has %d: %v", dir, m.HiddenTests, len(names), names)
	}
	return Task{Slug: m.Slug, Title: m.Title, Language: m.Language, Difficulty: m.Difficulty, Image: m.Image, RunCmd: m.RunCmd,
		SandboxTimeoutS: m.SandboxTimeoutS, HiddenTests: m.HiddenTests, TaskMD: string(md), RepoTar: repoTar, HiddenTar: hiddenTar}, nil
}

// Sync upserts the catalog; a task gone from it stays (past submissions reference it) but is deactivated
// and never picked as a daily task.
func Sync(ctx context.Context, pool *db.Pool, ts []Task) error {
	if len(ts) == 0 {
		return fmt.Errorf("tasks: catalog has no tasks, refusing to deactivate every task")
	}
	slugs := make([]string, 0, len(ts))
	return pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, t := range ts {
			if _, err := tx.Exec(ctx, `
				INSERT INTO tasks (slug, title, language, difficulty, task_md, repo_tar, hidden_tar, image, run_cmd, sandbox_timeout_s, hidden_tests, active, synced_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, true, now())
				ON CONFLICT (slug) DO UPDATE SET title = $2, language = $3, difficulty = $4, task_md = $5, repo_tar = $6, hidden_tar = $7,
				    image = $8, run_cmd = $9, sandbox_timeout_s = $10, hidden_tests = $11, active = true, synced_at = now()`,
				t.Slug, t.Title, t.Language, t.Difficulty, t.TaskMD, t.RepoTar, t.HiddenTar, t.Image, t.RunCmd, t.SandboxTimeoutS, t.HiddenTests); err != nil {
				return fmt.Errorf("tasks: sync %s: %w", t.Slug, err)
			}
			slugs = append(slugs, t.Slug)
		}
		_, err := tx.Exec(ctx, `UPDATE tasks SET active = false WHERE active AND NOT (slug = ANY($1))`, slugs)
		return err
	})
}

// TarDir packs dir into a deterministic gzip tarball with paths relative to dir.
func TarDir(dir string) ([]byte, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("tasks: walk %s: %w", dir, err)
	}
	files := make(map[string][]byte, len(paths))
	for _, p := range paths {
		rel, _ := filepath.Rel(dir, p)
		body, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		files[filepath.ToSlash(rel)] = body
	}
	return TarFiles(files)
}

// TarFiles packs files (path relative to the tarball root, mapped to contents) into a deterministic gzip
// tarball: sorted by path, zero mtimes, mode 0644.
func TarFiles(files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		body := files[name]
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(name), Mode: 0o644, Size: int64(len(body))}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(body); err != nil {
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

// ReadTar returns the regular files of a tarball made by TarFiles, keyed by path.
func ReadTar(data []byte) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("tasks: untar: %w", err)
	}
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("tasks: untar: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(tr, 16<<20))
		if err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(filepath.ToSlash(filepath.Clean(h.Name)), "./")] = body
	}
}

// Untar extracts a tarball made by TarFiles into dst, refusing entries that would escape dst.
func Untar(data []byte, dst string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("tasks: untar: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tasks: untar: %w", err)
		}
		clean := filepath.Clean(h.Name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("tasks: untar: illegal path %q", h.Name)
		}
		target := filepath.Join(dst, clean)
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, io.LimitReader(tr, 16<<20)); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
}
