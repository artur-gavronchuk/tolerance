package builds

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/sanitize"
	"tolerance/internal/submissions"
)

const (
	MaxUploadBytes = 5 << 20
	maxMadeWith    = 100
	// galleryLimit caps the gallery; the page is one screen of cards, not an archive.
	galleryLimit = 200
)

type Service struct {
	pool *db.Pool
	now  func() time.Time

	mu    sync.Mutex
	sites map[string]siteCache // entry id -> unpacked files of its current upload
}

type siteCache struct {
	version int
	files   map[string][]byte
}

func NewService(pool *db.Pool) *Service {
	return &Service{pool: pool, now: time.Now, sites: map[string]siteCache{}}
}

// ChallengeView is a challenge as the page shows it.
type ChallengeView struct {
	Challenge
	TaskMD        string     `json:"task_md"`
	ScenarioCount int        `json:"scenario_count"`
	Ends          *time.Time `json:"ends"` // when the next challenge takes over the page; nil when none is planned
	Current       bool       `json:"current"`
	Entries       int        `json:"entries"`
}

type ChallengeRef struct {
	Slug    string    `json:"slug"`
	Title   string    `json:"title"`
	TitleRu string    `json:"title_ru"`
	Starts  time.Time `json:"starts"`
	Current bool      `json:"current"`
}

// Entry is one person's upload. Checks is the score breakdown (see score.go), {} until scored.
type Entry struct {
	ID            string          `json:"id"`
	Handle        string          `json:"handle"`
	MadeWith      string          `json:"made_with"`
	Status        string          `json:"status"`
	Score         *int            `json:"score"`
	Checks        json.RawMessage `json:"checks"`
	FailureReason *string         `json:"failure_reason"`
	LogTail       string          `json:"log_tail,omitempty"` // the owner only
	HasShot       bool            `json:"has_shot"`
	Version       int             `json:"version"` // bumps on every re-upload (cache key for the screenshot)
	Votes         int             `json:"votes"`
	Voted         bool            `json:"voted"`
	Mine          bool            `json:"mine"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type Page struct {
	Challenge ChallengeView  `json:"challenge"`
	All       []ChallengeRef `json:"all"`
	Entries   []Entry        `json:"entries"`
	Mine      *Entry         `json:"mine"`
	Now       time.Time      `json:"now"`
}

func notFound() error { return httpx.NotFound() }

func invalid(msg string) error {
	return httpx.WithField(http.StatusUnprocessableEntity, "invalid_upload", msg, "file", "invalid")
}

// entryCols expects aliases e (build_entries), u (users) and $1 = the viewer's user id (” when anonymous).
const entryCols = `e.id, u.handle, e.made_with, e.status, e.score, e.checks, e.failure_reason, e.log_tail, e.shot IS NOT NULL, e.uploads,
	(SELECT count(*) FROM build_votes v WHERE v.entry_id = e.id),
	EXISTS (SELECT 1 FROM build_votes v WHERE v.entry_id = e.id AND v.user_id = $1), e.user_id = $1, e.updated_at`

type scanner interface{ Scan(...any) error }

func scanEntry(row scanner) (Entry, error) {
	var e Entry
	var checks []byte
	if err := row.Scan(&e.ID, &e.Handle, &e.MadeWith, &e.Status, &e.Score, &checks, &e.FailureReason, &e.LogTail, &e.HasShot, &e.Version,
		&e.Votes, &e.Voted, &e.Mine, &e.UpdatedAt); err != nil {
		return Entry{}, err
	}
	e.UpdatedAt = e.UpdatedAt.UTC()
	e.Checks = checks
	if !e.Mine {
		e.LogTail = ""
	}
	return e, nil
}

// Page is everything the build page shows for a challenge ("" = the current one).
func (s *Service) Page(ctx context.Context, slug, viewer string) (Page, error) {
	now := s.now().UTC()
	all := started(now)
	if len(all) == 0 {
		return Page{}, notFound()
	}
	c := all[0]
	if slug != "" {
		if c = find(slug, now); c == nil {
			return Page{}, notFound()
		}
	}
	p := Page{Now: now, Entries: []Entry{}}
	p.Challenge = ChallengeView{Challenge: *c, TaskMD: c.TaskMD, ScenarioCount: len(c.Scenarios), Current: c == all[0]}
	if n := next(now); n != nil && p.Challenge.Current {
		t := n.Starts
		p.Challenge.Ends = &t
	}
	for _, o := range all {
		p.All = append(p.All, ChallengeRef{Slug: o.Slug, Title: o.Title, TitleRu: o.TitleRu, Starts: o.Starts, Current: o == all[0]})
	}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+entryCols+` FROM build_entries e JOIN users u ON u.id = e.user_id
			WHERE e.challenge = $2 AND e.status = 'done' AND e.score IS NOT NULL AND u.banned_at IS NULL
			ORDER BY (SELECT count(*) FROM build_votes v WHERE v.entry_id = e.id) DESC, e.score DESC, e.finished_at
			LIMIT $3`, viewer, c.Slug, galleryLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			e, err := scanEntry(rows)
			if err != nil {
				return err
			}
			p.Entries = append(p.Entries, e)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM build_entries e JOIN users u ON u.id = e.user_id
			WHERE e.challenge = $1 AND e.status = 'done' AND u.banned_at IS NULL`, c.Slug).Scan(&p.Challenge.Entries); err != nil {
			return err
		}
		if viewer == "" {
			return nil
		}
		e, err := scanEntry(tx.QueryRow(ctx, `SELECT `+entryCols+` FROM build_entries e JOIN users u ON u.id = e.user_id
			WHERE e.challenge = $2 AND e.user_id = $1`, viewer, c.Slug))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		p.Mine = &e
		return nil
	})
	return p, err
}

// Create stores an upload as the person's entry for the challenge (replacing the previous one) and queues it.
// A single .html file is accepted as the site's index.html.
func (s *Service) Create(ctx context.Context, userID, slug, filename string, data []byte, madeWith string) (Entry, error) {
	c := find(slug, s.now().UTC())
	if c == nil {
		return Entry{}, notFound()
	}
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm") {
		z, err := singleFileZip("index.html", data)
		if err != nil {
			return Entry{}, err
		}
		data = z
	}
	files, err := submissions.ReadZip(data)
	if err != nil {
		return Entry{}, err
	}
	if _, ok := files["index.html"]; !ok {
		return Entry{}, invalid("The zip needs an index.html at its root (or upload a single .html file)")
	}
	madeWith = sanitize.CleanText(madeWith, maxMadeWith)
	var id string
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO build_entries (id, challenge, user_id, zip, made_with) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (challenge, user_id) DO UPDATE SET zip = EXCLUDED.zip, made_with = EXCLUDED.made_with,
				status = 'queued', score = NULL, checks = '{}'::jsonb, shot = NULL, log_tail = '', failure_reason = NULL,
				uploads = build_entries.uploads + 1, updated_at = now(), finished_at = NULL
			RETURNING id`, idgen.New("bld"), c.Slug, userID, data, madeWith).Scan(&id)
	})
	if err != nil {
		return Entry{}, err
	}
	return s.entry(ctx, id, userID)
}

func singleFileZip(name string, body []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(body); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *Service) entry(ctx context.Context, id, viewer string) (Entry, error) {
	var e Entry
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		e, err = scanEntry(tx.QueryRow(ctx, `SELECT `+entryCols+` FROM build_entries e JOIN users u ON u.id = e.user_id
			WHERE e.id = $2`, viewer, id))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Entry{}, notFound()
	}
	return e, err
}

// Vote adds (on) or removes the viewer's vote for a scored entry that is not theirs.
func (s *Service) Vote(ctx context.Context, userID, entryID string, on bool) (Entry, error) {
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var owner, status string
		err := tx.QueryRow(ctx, `SELECT user_id, status FROM build_entries WHERE id = $1`, entryID).Scan(&owner, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		}
		if err != nil {
			return err
		}
		if owner == userID {
			return httpx.StateConflict("You can't vote for your own entry")
		}
		if !on {
			_, err = tx.Exec(ctx, `DELETE FROM build_votes WHERE entry_id = $1 AND user_id = $2`, entryID, userID)
			return err
		}
		if status != "done" {
			return httpx.StateConflict("This entry is not scored yet")
		}
		_, err = tx.Exec(ctx, `INSERT INTO build_votes (entry_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, entryID, userID)
		return err
	})
	if err != nil {
		return Entry{}, err
	}
	return s.entry(ctx, entryID, userID)
}

// Delete removes an entry: its owner, or an admin (spam).
func (s *Service) Delete(ctx context.Context, userID string, admin bool, entryID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM build_entries WHERE id = $1 AND (user_id = $2 OR $3)`, entryID, userID, admin)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return notFound()
		}
		return nil
	})
}

// SiteFile returns one file of an entry's current upload for the preview; dirs resolve to their index.html.
func (s *Service) SiteFile(ctx context.Context, entryID, raw string) (string, []byte, error) {
	var version int
	var data []byte
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT e.uploads FROM build_entries e JOIN users u ON u.id = e.user_id
			WHERE e.id = $1 AND u.banned_at IS NULL`, entryID).Scan(&version)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, notFound()
	}
	if err != nil {
		return "", nil, err
	}
	s.mu.Lock()
	cached, ok := s.sites[entryID]
	s.mu.Unlock()
	files := cached.files
	if !ok || cached.version != version {
		err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT zip, uploads FROM build_entries WHERE id = $1`, entryID).Scan(&data, &version)
		})
		if err != nil {
			return "", nil, err
		}
		if files, err = submissions.ReadZip(data); err != nil {
			return "", nil, notFound()
		}
		s.mu.Lock()
		if len(s.sites) >= 64 {
			clear(s.sites)
		}
		s.sites[entryID] = siteCache{version: version, files: files}
		s.mu.Unlock()
	}
	p := strings.TrimPrefix(path.Clean("/"+raw), "/")
	if p == "" || strings.HasSuffix(raw, "/") {
		p = path.Join(p, "index.html")
	}
	body, ok := files[p]
	if !ok {
		p = path.Join(p, "index.html")
		body, ok = files[p]
	}
	if !ok {
		return "", nil, notFound()
	}
	return p, body, nil
}

// Shot is an entry's screenshot (JPEG).
func (s *Service) Shot(ctx context.Context, entryID string) ([]byte, error) {
	var shot []byte
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT e.shot FROM build_entries e JOIN users u ON u.id = e.user_id
			WHERE e.id = $1 AND u.banned_at IS NULL`, entryID).Scan(&shot)
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && shot == nil) {
		return nil, notFound()
	}
	return shot, err
}

// TaskZip is the download: <slug>/TASK.md and a README on how to hand it in.
func (s *Service) TaskZip(slug, publicURL string) ([]byte, error) {
	c := find(slug, s.now().UTC())
	if c == nil {
		return nil, notFound()
	}
	if publicURL == "" {
		publicURL = "https://tolerance.cc"
	}
	readme := "# " + c.Title + "\n\n" +
		"Give TASK.md to your coding agent and let it build the site.\n\n" +
		"Hand-in: a zip with `index.html` at its root (plus any CSS, JS and images), or a single `.html` file,\n" +
		"up to 5 MB, static files only, no network. Upload it at " + strings.TrimRight(publicURL, "/") + "/?c=" + c.Slug + "\n"
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct{ name, body string }{{"TASK.md", c.TaskMD}, {"README.md", readme}} {
		w, err := zw.Create(c.Slug + "/" + f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
