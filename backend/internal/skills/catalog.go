// Package skills holds the qualification catalog: skills and their hidden
// task pools, loaded from fixtures/skills the same way proofs loads its
// catalog.
package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/proofs"
)

type Skill struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Language    string `json:"language"`
	Image       string `json:"image"`
	RunCmd      string `json:"run_cmd"`
	Description string `json:"description"`
}

type Task struct {
	Slug            string
	SkillSlug       string
	Title           string
	Difficulty      int
	AgentTimeoutS   int
	SandboxTimeoutS int
	HiddenTests     int
	TaskMD          string
	RepoTar         []byte
	HiddenTar       []byte
	RepoSHA256      string
}

type taskManifest struct {
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	Difficulty      int    `json:"difficulty"`
	AgentTimeoutS   int    `json:"agent_timeout_s"`
	SandboxTimeoutS int    `json:"sandbox_timeout_s"`
	HiddenTests     int    `json:"hidden_tests"`
}

// LoadCatalog reads fixtures/skills/<skill>/skill.json and every task
// directory beside it (a directory containing manifest.json).
func LoadCatalog(dir string) ([]Skill, []Task, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("skills: read %s: %w", dir, err)
	}
	var skills []Skill
	var tasks []Task
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sdir := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(filepath.Join(sdir, "skill.json"))
		if err != nil {
			return nil, nil, fmt.Errorf("skills: %s: %w", sdir, err)
		}
		var s Skill
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, nil, fmt.Errorf("skills: %s/skill.json: %w", sdir, err)
		}
		if s.Slug == "" || s.Image == "" || s.RunCmd == "" || s.Language == "" {
			return nil, nil, fmt.Errorf("skills: %s/skill.json: slug, language, image, run_cmd required", sdir)
		}
		skills = append(skills, s)
		sub, err := os.ReadDir(sdir)
		if err != nil {
			return nil, nil, err
		}
		for _, te := range sub {
			if !te.IsDir() {
				continue
			}
			tdir := filepath.Join(sdir, te.Name())
			if _, err := os.Stat(filepath.Join(tdir, "manifest.json")); err != nil {
				continue
			}
			t, err := LoadTask(tdir, s.Slug, s.Language)
			if err != nil {
				return nil, nil, err
			}
			tasks = append(tasks, t)
		}
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Slug < skills[j].Slug })
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Slug < tasks[j].Slug })
	return skills, tasks, nil
}

// LoadTask reads one task directory (manifest.json, TASK.md, repo/, _hidden/).
func LoadTask(dir, skill, language string) (Task, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Task{}, err
	}
	var m taskManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Task{}, fmt.Errorf("skills: %s/manifest.json: %w", dir, err)
	}
	if m.Slug == "" || m.Difficulty < 1 || m.Difficulty > 3 || m.HiddenTests <= 0 || m.AgentTimeoutS <= 0 || m.SandboxTimeoutS <= 0 {
		return Task{}, fmt.Errorf("skills: %s/manifest.json: slug, difficulty 1-3, hidden_tests, timeouts required", dir)
	}
	md, err := os.ReadFile(filepath.Join(dir, "TASK.md"))
	if err != nil {
		return Task{}, err
	}
	repoTar, err := proofs.TarDir(filepath.Join(dir, "repo"))
	if err != nil {
		return Task{}, err
	}
	hiddenTar, err := proofs.TarDir(filepath.Join(dir, "_hidden"))
	if err != nil {
		return Task{}, err
	}
	// The score divides by hidden_tests and counts hidden tests by name, so
	// the manifest and the files must agree on how many there are.
	names, err := proofs.HiddenTestNames(language, hiddenTar)
	if err != nil {
		return Task{}, err
	}
	if len(names) != m.HiddenTests {
		return Task{}, fmt.Errorf("skills: %s: manifest says %d hidden tests, _hidden has %d: %v", dir, m.HiddenTests, len(names), names)
	}
	sum := sha256.Sum256(repoTar)
	return Task{Slug: m.Slug, SkillSlug: skill, Title: m.Title, Difficulty: m.Difficulty, AgentTimeoutS: m.AgentTimeoutS,
		SandboxTimeoutS: m.SandboxTimeoutS, HiddenTests: m.HiddenTests, TaskMD: string(md), RepoTar: repoTar, HiddenTar: hiddenTar,
		RepoSHA256: hex.EncodeToString(sum[:])}, nil
}

func SyncCatalog(ctx context.Context, pool *db.Pool, skills []Skill, tasks []Task) error {
	slugs := make([]string, 0, len(tasks))
	return pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, s := range skills {
			if _, err := tx.Exec(ctx, `INSERT INTO skills (slug, title, language, image, run_cmd, description, updated_at) VALUES ($1,$2,$3,$4,$5,$6, now())
				ON CONFLICT (slug) DO UPDATE SET title = $2, language = $3, image = $4, run_cmd = $5, description = $6, updated_at = now()`,
				s.Slug, s.Title, s.Language, s.Image, s.RunCmd, s.Description); err != nil {
				return fmt.Errorf("skills: sync %s: %w", s.Slug, err)
			}
		}
		for _, t := range tasks {
			if _, err := tx.Exec(ctx, `INSERT INTO skill_tasks (slug, skill_slug, title, difficulty, agent_timeout_s, sandbox_timeout_s, hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, now())
				ON CONFLICT (slug) DO UPDATE SET skill_slug = $2, title = $3, difficulty = $4, agent_timeout_s = $5, sandbox_timeout_s = $6, hidden_tests = $7,
				  task_md = $8, repo_tar = $9, hidden_tar = $10, repo_sha256 = $11, active = true, updated_at = now()`,
				t.Slug, t.SkillSlug, t.Title, t.Difficulty, t.AgentTimeoutS, t.SandboxTimeoutS, t.HiddenTests, t.TaskMD, t.RepoTar, t.HiddenTar, t.RepoSHA256); err != nil {
				return fmt.Errorf("skills: sync task %s: %w", t.Slug, err)
			}
			slugs = append(slugs, t.Slug)
		}
		// A task gone from the catalog stays (past proofs reference it) but is
		// never picked for a new run.
		_, err := tx.Exec(ctx, `UPDATE skill_tasks SET active = false, updated_at = now() WHERE active AND NOT (slug = ANY($1))`, slugs)
		return err
	})
}

// HiddenNamesByTask maps every task slug to the hidden test names the
// sandbox must report for it (proofs.HiddenTestNames in the language of the
// task's skill). Tests and the qualification service use it to know which
// names count toward a score.
func HiddenNamesByTask(sk []Skill, tasks []Task) (map[string][]string, error) {
	lang := make(map[string]string, len(sk))
	for _, s := range sk {
		lang[s.Slug] = s.Language
	}
	out := make(map[string][]string, len(tasks))
	for _, t := range tasks {
		l, ok := lang[t.SkillSlug]
		if !ok {
			return nil, fmt.Errorf("skills: task %s: unknown skill %q", t.Slug, t.SkillSlug)
		}
		names, err := proofs.HiddenTestNames(l, t.HiddenTar)
		if err != nil {
			return nil, fmt.Errorf("skills: task %s: %w", t.Slug, err)
		}
		out[t.Slug] = names
	}
	return out, nil
}
