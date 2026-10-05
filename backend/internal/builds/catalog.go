// Package builds runs build challenges: the platform gives a task ("build a kanban board"), people have their
// agent build it as a static site and upload a zip; the sandbox opens the site in Chromium, takes a screenshot
// and scores it (acceptance scenarios + quality signals), and every scored entry goes into a public gallery
// with a live preview where people vote. One challenge is current at a time (the latest that has started);
// earlier ones stay open.
package builds

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"time"
)

// The catalog: catalog/<slug>/{manifest.json, TASK.md, scenarios.json}. Names starting with _ (reference
// solutions) are not embedded.
//
//go:embed catalog
var catalogFS embed.FS

type Challenge struct {
	Slug      string            `json:"slug"`
	Title     string            `json:"title"`
	TitleRu   string            `json:"title_ru"`
	Summary   string            `json:"summary"`
	SummaryRu string            `json:"summary_ru"`
	Starts    time.Time         `json:"starts"`
	TimeoutS  int               `json:"-"`
	TaskMD    string            `json:"-"`
	Scenarios []json.RawMessage `json:"-"`
}

type manifest struct {
	Title     string `json:"title"`
	TitleRu   string `json:"title_ru"`
	Summary   string `json:"summary"`
	SummaryRu string `json:"summary_ru"`
	Starts    string `json:"starts"` // YYYY-MM-DD, UTC
	TimeoutS  int    `json:"timeout_s"`
}

// catalog is sorted by start date, oldest first.
var catalog = mustLoad(catalogFS)

func mustLoad(fsys fs.FS) []Challenge {
	cs, err := load(fsys)
	if err != nil {
		panic(err)
	}
	return cs
}

func load(fsys fs.FS) ([]Challenge, error) {
	dirs, err := fs.Glob(fsys, "catalog/*/manifest.json")
	if err != nil {
		return nil, err
	}
	var out []Challenge
	for _, mf := range dirs {
		dir := path.Dir(mf)
		raw, err := fs.ReadFile(fsys, mf)
		if err != nil {
			return nil, err
		}
		var m manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("builds: %s: %w", mf, err)
		}
		starts, err := time.Parse(time.DateOnly, m.Starts)
		if err != nil || m.Title == "" || m.TimeoutS <= 0 {
			return nil, fmt.Errorf("builds: %s: needs title, starts (YYYY-MM-DD) and timeout_s", mf)
		}
		md, err := fs.ReadFile(fsys, dir+"/TASK.md")
		if err != nil {
			return nil, err
		}
		c := Challenge{Slug: path.Base(dir), Title: m.Title, TitleRu: m.TitleRu, Summary: m.Summary, SummaryRu: m.SummaryRu,
			Starts: starts.UTC(), TimeoutS: m.TimeoutS, TaskMD: string(md), Scenarios: []json.RawMessage{}}
		if sc, err := fs.ReadFile(fsys, dir+"/scenarios.json"); err == nil {
			if err := json.Unmarshal(sc, &c.Scenarios); err != nil {
				return nil, fmt.Errorf("builds: %s/scenarios.json: %w", dir, err)
			}
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Starts.Before(out[j].Starts) })
	return out, nil
}

// started lists the challenges that have started by now, newest first; the first one is current.
func started(now time.Time) []*Challenge {
	var out []*Challenge
	for i := len(catalog) - 1; i >= 0; i-- {
		if !catalog[i].Starts.After(now) {
			out = append(out, &catalog[i])
		}
	}
	return out
}

// next is the first challenge that starts after now, or nil.
func next(now time.Time) *Challenge {
	for i := range catalog {
		if catalog[i].Starts.After(now) {
			return &catalog[i]
		}
	}
	return nil
}

// find returns a started challenge by slug.
func find(slug string, now time.Time) *Challenge {
	for _, c := range started(now) {
		if c.Slug == slug {
			return c
		}
	}
	return nil
}
