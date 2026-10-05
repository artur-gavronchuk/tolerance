// Package builds runs build challenges: the platform gives a contract ("a creature that lives in its URL"),
// people have their agent build it as a static site and upload a zip; the sandbox opens the site in Chromium
// and runs the challenge's hidden tests, people vote in a public gallery, and the final score is 60% tests +
// 40% votes. A season (catalog/season.json) opens the challenges one after another.
package builds

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"
)

// The public catalog: catalog/<slug>/{manifest.json, contract.en.md, contract.ru.md} plus catalog/season.json.
// Hidden tests are not here (this repository is public): LoadTests reads them from the private catalog.
// Names starting with _ are not embedded.
//
//go:embed catalog
var catalogFS embed.FS

const (
	FormatGame   = "game"
	FormatSite   = "site"
	FormatMobile = "mobile"
	FormatTool   = "tool"

	StatusUpcoming = "upcoming"
	StatusOpen     = "open"
	StatusClosed   = "closed"
)

// Text is a string in both interface languages.
type Text struct {
	En string `json:"en"`
	Ru string `json:"ru"`
}

type Challenge struct {
	Slug     string    `json:"slug"`
	Format   string    `json:"format"`
	Title    Text      `json:"title"`
	OneLiner Text      `json:"one_liner"`
	OpensAt  time.Time `json:"opens_at"`
	ClosesAt time.Time `json:"closes_at"`

	Contract  Text              `json:"-"`
	Hidden    bool              `json:"-"` // kept with its entries, shown nowhere
	TimeoutS  int               `json:"-"`
	Scenarios []json.RawMessage `json:"-"` // the hidden tests; nil when none are configured
	Reference string            `json:"-"` // directory of the reference solution, "" when none
}

// Mobile challenges are tested and shown at phone width.
func (c *Challenge) Mobile() bool { return c.Format == FormatMobile }

func (c *Challenge) Status(now time.Time) string {
	switch {
	case now.Before(c.OpensAt):
		return StatusUpcoming
	case now.Before(c.ClosesAt):
		return StatusOpen
	default:
		return StatusClosed
	}
}

type manifest struct {
	Format   string `json:"format"`
	Title    Text   `json:"title"`
	OneLiner Text   `json:"one_liner"`
	TimeoutS int    `json:"timeout_s"`
	Hidden   bool   `json:"hidden"`
}

type season struct {
	Challenges []struct {
		Slug     string `json:"slug"`
		OpensAt  string `json:"opens_at"`  // YYYY-MM-DD, 00:00 UTC
		ClosesAt string `json:"closes_at"` // YYYY-MM-DD, 00:00 UTC
	} `json:"challenges"`
}

// catalog holds the season's challenges in season order, then the hidden ones.
var catalog = mustLoad(catalogFS)

func mustLoad(fsys fs.FS) []Challenge {
	cs, err := load(fsys)
	if err != nil {
		panic(err)
	}
	return cs
}

func load(fsys fs.FS) ([]Challenge, error) {
	manifests, err := fs.Glob(fsys, "catalog/*/manifest.json")
	if err != nil {
		return nil, err
	}
	bySlug := map[string]*Challenge{}
	var hidden []Challenge
	for _, mf := range manifests {
		dir := path.Dir(mf)
		raw, err := fs.ReadFile(fsys, mf)
		if err != nil {
			return nil, err
		}
		var m manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("builds: %s: %w", mf, err)
		}
		switch m.Format {
		case FormatGame, FormatSite, FormatMobile, FormatTool:
		default:
			return nil, fmt.Errorf("builds: %s: format must be game, site, mobile or tool", mf)
		}
		if m.Title.En == "" || m.OneLiner.En == "" || m.TimeoutS <= 0 {
			return nil, fmt.Errorf("builds: %s: needs title.en, one_liner.en and timeout_s", mf)
		}
		en, err := fs.ReadFile(fsys, dir+"/contract.en.md")
		if err != nil {
			return nil, fmt.Errorf("builds: %s: %w", dir, err)
		}
		ru, _ := fs.ReadFile(fsys, dir+"/contract.ru.md")
		c := Challenge{Slug: path.Base(dir), Format: m.Format, Title: m.Title, OneLiner: m.OneLiner, Hidden: m.Hidden,
			TimeoutS: m.TimeoutS, Contract: Text{En: string(en), Ru: string(ru)}}
		// Retired challenges kept their tests in the public catalog; they are hidden anyway.
		if sc, err := fs.ReadFile(fsys, dir+"/scenarios.json"); err == nil {
			if err := json.Unmarshal(sc, &c.Scenarios); err != nil {
				return nil, fmt.Errorf("builds: %s/scenarios.json: %w", dir, err)
			}
		}
		if c.Hidden {
			hidden = append(hidden, c)
		} else {
			bySlug[c.Slug] = &c
		}
	}

	raw, err := fs.ReadFile(fsys, "catalog/season.json")
	if err != nil {
		return nil, err
	}
	var s season
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("builds: season.json: %w", err)
	}
	var out []Challenge
	for _, e := range s.Challenges {
		c, ok := bySlug[e.Slug]
		if !ok {
			return nil, fmt.Errorf("builds: season.json: %q is not a visible challenge", e.Slug)
		}
		delete(bySlug, e.Slug)
		opens, err1 := time.Parse(time.DateOnly, e.OpensAt)
		closes, err2 := time.Parse(time.DateOnly, e.ClosesAt)
		if err1 != nil || err2 != nil || !opens.Before(closes) {
			return nil, fmt.Errorf("builds: season.json: %q needs opens_at < closes_at (YYYY-MM-DD)", e.Slug)
		}
		c.OpensAt, c.ClosesAt = opens.UTC(), closes.UTC()
		out = append(out, *c)
	}
	for slug := range bySlug {
		return nil, fmt.Errorf("builds: %q is neither in season.json nor hidden", slug)
	}
	return append(out, hidden...), nil
}

// LoadTests reads the private catalog: <dir>/<slug>/tests.json (the hidden tests) and <dir>/<slug>/reference/
// (the reference solution, seeded as the platform's own entry). Call it once at start, before serving.
// Challenges it has nothing for keep what they had (nothing, for a season challenge).
func LoadTests(dir string) (int, error) {
	if dir == "" {
		return 0, nil
	}
	n := 0
	for i := range catalog {
		c := &catalog[i]
		raw, err := os.ReadFile(filepath.Join(dir, c.Slug, "tests.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return n, err
		}
		var sc []json.RawMessage
		if err := json.Unmarshal(raw, &sc); err != nil || len(sc) == 0 {
			return n, fmt.Errorf("builds: %s/tests.json: a non-empty array of scenarios is required", c.Slug)
		}
		c.Scenarios = sc
		if st, err := os.Stat(filepath.Join(dir, c.Slug, "reference", "index.html")); err == nil && !st.IsDir() {
			c.Reference = filepath.Join(dir, c.Slug, "reference")
		}
		n++
	}
	return n, nil
}

// visible lists the challenges people can see, in season order.
func visible() []*Challenge {
	var out []*Challenge
	for i := range catalog {
		if !catalog[i].Hidden {
			out = append(out, &catalog[i])
		}
	}
	return out
}

// find returns a visible challenge by slug.
func find(slug string) *Challenge {
	for _, c := range visible() {
		if c.Slug == slug {
			return c
		}
	}
	return nil
}

// bySlug returns any challenge, hidden ones included (the worker scores whatever was uploaded).
func bySlug(slug string) *Challenge {
	for i := range catalog {
		if catalog[i].Slug == slug {
			return &catalog[i]
		}
	}
	return nil
}
