package proofs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

type manifest struct {
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	Language        string `json:"language"`
	Kind            string `json:"kind"`
	Image           string `json:"image"`
	RunCmd          string `json:"run_cmd"`
	AgentTimeoutS   int    `json:"agent_timeout_s"`
	SandboxTimeoutS int    `json:"sandbox_timeout_s"`
	VisibleTests    int    `json:"visible_tests"`
	HiddenTests     int    `json:"hidden_tests"`
}

// LoadCatalog reads every task directory under dir: manifest.json, TASK.md,
// repo/ (what the agent gets) and _hidden/ (copied over repo/ before the
// sandbox run). The underscore keeps the Go toolchain out of _hidden.
func LoadCatalog(dir string) ([]Task, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("proofs: read catalog %s: %w", dir, err)
	}
	var out []Task
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t, err := loadTask(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

func loadTask(dir string) (Task, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Task{}, fmt.Errorf("proofs: %s: %w", dir, err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Task{}, fmt.Errorf("proofs: %s/manifest.json: %w", dir, err)
	}
	if m.Slug == "" || m.Image == "" || m.RunCmd == "" || m.AgentTimeoutS <= 0 || m.SandboxTimeoutS <= 0 {
		return Task{}, fmt.Errorf("proofs: %s/manifest.json: slug, image, run_cmd and timeouts are required", dir)
	}
	taskMD, err := os.ReadFile(filepath.Join(dir, "TASK.md"))
	if err != nil {
		return Task{}, fmt.Errorf("proofs: %s: %w", dir, err)
	}
	repoTar, err := TarDir(filepath.Join(dir, "repo"))
	if err != nil {
		return Task{}, err
	}
	hiddenTar, err := TarDir(filepath.Join(dir, "_hidden"))
	if err != nil {
		return Task{}, err
	}
	kind := m.Kind
	if kind == "" {
		kind = KindProof
	}
	sum := sha256.Sum256(repoTar)
	return Task{Slug: m.Slug, Title: m.Title, Language: m.Language, Kind: kind, Image: m.Image, RunCmd: m.RunCmd,
		AgentTimeoutS: m.AgentTimeoutS, SandboxTimeoutS: m.SandboxTimeoutS, VisibleTests: m.VisibleTests, HiddenTests: m.HiddenTests,
		TaskMD: string(taskMD), RepoTar: repoTar, HiddenTar: hiddenTar, RepoSHA256: hex.EncodeToString(sum[:])}, nil
}

// TarDir packs dir into a deterministic gzip tarball with paths relative to
// dir. Deterministic (sorted, zero mtimes) so the sha256 is stable across
// deploys and the connector can verify it.
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
		return nil, fmt.Errorf("proofs: walk %s: %w", dir, err)
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
// tarball: sorted by path, zero mtimes, mode 0644. A nil or empty map produces a valid, empty tarball.
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

// Untar extracts a tarball produced by TarDir into dst, refusing entries
// that would escape dst.
func Untar(data []byte, dst string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("proofs: untar: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("proofs: untar: %w", err)
		}
		clean := filepath.Clean(h.Name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("proofs: untar: illegal path %q", h.Name)
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

// SyncCatalog upserts tasks into proof_tasks. Run by cmd/migrate.
func SyncCatalog(ctx context.Context, pool *db.Pool, tasks []Task) error {
	return pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, t := range tasks {
			kind := t.Kind
			if kind == "" {
				kind = KindProof
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO proof_tasks (slug, title, language, kind, image, run_cmd, agent_timeout_s, sandbox_timeout_s,
				    visible_tests, hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14, now())
				ON CONFLICT (slug) DO UPDATE SET title = $2, language = $3, kind = $4, image = $5, run_cmd = $6, agent_timeout_s = $7,
				    sandbox_timeout_s = $8, visible_tests = $9, hidden_tests = $10, task_md = $11, repo_tar = $12,
				    hidden_tar = $13, repo_sha256 = $14, updated_at = now()`,
				t.Slug, t.Title, t.Language, kind, t.Image, t.RunCmd, t.AgentTimeoutS, t.SandboxTimeoutS,
				t.VisibleTests, t.HiddenTests, t.TaskMD, t.RepoTar, t.HiddenTar, t.RepoSHA256)
			if err != nil {
				return fmt.Errorf("proofs: sync %s: %w", t.Slug, err)
			}
		}
		return nil
	})
}
