// Package tasks is the catalog of coding tasks: loaded from fixtures on disk by cmd/migrate, stored in the
// tasks table, and served to people as a repo zip. A task is a small repository with a bug or a missing
// feature, a TASK.md, and hidden tests the platform runs over the result.
package tasks

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
)

// KindOptimize is the manifest kind of a scored optimization task; the default kind is bugfix.
const KindOptimize = "optimize"

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
	Kind            string // bugfix | optimize
	Direction       string // optimize: max | min
	SolveCmd        string
	CaseTimeLimitS  int
	Cases           int
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
	Kind            string `json:"kind"`
	Direction       string `json:"direction"`
	SolveCmd        string `json:"solve_cmd"`
	CaseTimeLimitS  int    `json:"case_time_limit_s"`
	Cases           int    `json:"cases"`
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
	if m.Kind == "" {
		m.Kind = "bugfix"
	}
	if m.Kind != "bugfix" && m.Kind != "optimize" {
		return Task{}, fmt.Errorf("tasks: %s/manifest.json: unknown kind %q", dir, m.Kind)
	}
	if m.Kind == KindOptimize && m.RunCmd == "" {
		m.RunCmd = m.SolveCmd
	}
	if m.Slug == "" || m.Language == "" || m.Image == "" || m.RunCmd == "" || m.SandboxTimeoutS <= 0 || m.Difficulty < 1 || m.Difficulty > 3 {
		return Task{}, fmt.Errorf("tasks: %s/manifest.json: slug, language, image, run_cmd, sandbox_timeout_s and difficulty 1-3 are required", dir)
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
	t := Task{Slug: m.Slug, Title: m.Title, Language: m.Language, Difficulty: m.Difficulty, Image: m.Image, RunCmd: m.RunCmd,
		SandboxTimeoutS: m.SandboxTimeoutS, Kind: m.Kind, TaskMD: string(md), RepoTar: repoTar, HiddenTar: hiddenTar}
	if m.Kind == KindOptimize {
		if (m.Direction != "max" && m.Direction != "min") || m.SolveCmd == "" || m.CaseTimeLimitS <= 0 || m.Cases <= 0 {
			return Task{}, fmt.Errorf("tasks: %s/manifest.json: optimize needs direction (max|min), solve_cmd, case_time_limit_s and cases", dir)
		}
		names, err := HiddenCaseNames(hiddenTar)
		if err != nil {
			return Task{}, fmt.Errorf("tasks: %s: %w", dir, err)
		}
		if len(names) != m.Cases {
			return Task{}, fmt.Errorf("tasks: %s: manifest says %d cases, _hidden/cases has %d", dir, m.Cases, len(names))
		}
		t.Direction, t.SolveCmd, t.CaseTimeLimitS, t.Cases, t.HiddenTests = m.Direction, m.SolveCmd, m.CaseTimeLimitS, m.Cases, m.Cases
		return t, nil
	}
	if m.HiddenTests <= 0 {
		return Task{}, fmt.Errorf("tasks: %s/manifest.json: hidden_tests is required", dir)
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
	t.HiddenTests = m.HiddenTests
	return t, nil
}

// Sync upserts the catalog; a task gone from it stays (past submissions reference it) but is deactivated
// and never picked as a daily task.
func Sync(ctx context.Context, pool *db.Pool, ts []Task) error {
	if len(ts) == 0 {
		return fmt.Errorf("tasks: catalog has no tasks, refusing to deactivate every task")
	}
	slugs := make([]string, 0, len(ts))
	return pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := checkPlayedUnchanged(ctx, tx, ts); err != nil {
			return err
		}
		for _, t := range ts {
			if _, err := tx.Exec(ctx, `
				INSERT INTO tasks (slug, title, language, difficulty, task_md, repo_tar, hidden_tar, image, run_cmd, sandbox_timeout_s, hidden_tests,
				    kind, direction, solve_cmd, case_time_limit_s, cases, active, synced_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,''),$14,$15,$16, true, now())
				ON CONFLICT (slug) DO UPDATE SET title = $2, language = $3, difficulty = $4, task_md = $5, repo_tar = $6, hidden_tar = $7,
				    image = $8, run_cmd = $9, sandbox_timeout_s = $10, hidden_tests = $11,
				    kind = $12, direction = NULLIF($13,''), solve_cmd = $14, case_time_limit_s = $15, cases = $16, active = true, synced_at = now()`,
				t.Slug, t.Title, t.Language, t.Difficulty, t.TaskMD, t.RepoTar, t.HiddenTar, t.Image, t.RunCmd, t.SandboxTimeoutS, t.HiddenTests,
				t.Kind, t.Direction, t.SolveCmd, t.CaseTimeLimitS, t.Cases); err != nil {
				return fmt.Errorf("tasks: sync %s: %w", t.Slug, err)
			}
			slugs = append(slugs, t.Slug)
		}
		_, err := tx.Exec(ctx, `UPDATE tasks SET active = false WHERE active AND NOT (slug = ANY($1))`, slugs)
		return err
	})
}

// checkPlayedUnchanged refuses a sync that would change what a played task (one that has been a daily
// task) checks against: its statement, repository, hidden tests and run parameters. Past verdicts, the
// archive and queued runs all refer to the task by slug, so a fix to a played task has to ship as a new
// slug. Title and difficulty are presentation and may change. The whole sync is rejected, so the catalog
// is never half-applied.
func checkPlayedUnchanged(ctx context.Context, tx pgx.Tx, ts []Task) error {
	rows, err := tx.Query(ctx, `
		SELECT t.slug, t.language, t.image, t.run_cmd, t.sandbox_timeout_s, t.hidden_tests, t.kind, coalesce(t.direction, ''),
		       t.solve_cmd, t.case_time_limit_s, t.cases, t.task_md, t.repo_tar, t.hidden_tar, min(d.day)::text
		FROM tasks t JOIN daily_tasks d ON d.task_slug = t.slug
		GROUP BY t.slug`)
	if err != nil {
		return err
	}
	played := map[string]struct {
		sum string
		day string
	}{}
	for rows.Next() {
		var t Task
		var day string
		if err := rows.Scan(&t.Slug, &t.Language, &t.Image, &t.RunCmd, &t.SandboxTimeoutS, &t.HiddenTests, &t.Kind, &t.Direction,
			&t.SolveCmd, &t.CaseTimeLimitS, &t.Cases, &t.TaskMD, &t.RepoTar, &t.HiddenTar, &day); err != nil {
			rows.Close()
			return err
		}
		played[t.Slug] = struct {
			sum string
			day string
		}{Fingerprint(t), day}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var changed []string
	for _, t := range ts {
		if p, ok := played[t.Slug]; ok && p.sum != Fingerprint(t) {
			changed = append(changed, fmt.Sprintf("%s (daily task on %s)", t.Slug, p.day))
		}
	}
	if len(changed) > 0 {
		return fmt.Errorf("refusing to sync: the checked content of played task(s) changed: %s; "+
			"a played task is frozen, publish the fix under a new slug", strings.Join(changed, ", "))
	}
	return nil
}

// Fingerprint is a SHA-256 over everything that decides a verdict for t: statement, repository, hidden
// tests and run parameters (not the title or difficulty). Each field is length-prefixed so no two
// different tasks can collide by shifting bytes between fields.
func Fingerprint(t Task) string {
	h := sha256.New()
	for _, f := range [][]byte{
		[]byte(t.Language), []byte(t.Image), []byte(t.RunCmd), []byte(strconv.Itoa(t.SandboxTimeoutS)),
		[]byte(strconv.Itoa(t.HiddenTests)), []byte(t.Kind), []byte(t.Direction), []byte(t.SolveCmd),
		[]byte(strconv.Itoa(t.CaseTimeLimitS)), []byte(strconv.Itoa(t.Cases)), []byte(t.TaskMD), t.RepoTar, t.HiddenTar,
	} {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(f)))
		h.Write(n[:])
		h.Write(f)
	}
	return hex.EncodeToString(h.Sum(nil))
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
