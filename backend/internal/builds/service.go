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

// ChallengeView is a challenge as its page shows it.
type ChallengeView struct {
	Challenge
	TaskMD        string `json:"task_md"`
	ScenarioCount int    `json:"scenario_count"`
	Entries       int    `json:"entries"`
}

// Card is a challenge on the home page: its numbers and its best entries.
type Card struct {
	Challenge
	Entries int     `json:"entries"`
	Votes   int     `json:"votes"`
	Top     []Entry `json:"top"` // up to 3, gallery order
	Mine    *Entry  `json:"mine"`
}

// Entry is one person's upload. Checks is the score breakdown (see score.go), {} until scored.
type Entry struct {
	ID            string          `json:"id"`
	Challenge     string          `json:"challenge"`
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
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type Page struct {
	Challenge ChallengeView `json:"challenge"`
	Entries   []Entry       `json:"entries"`
	Mine      *Entry        `json:"mine"`
}

// EntryPage is one solution with its challenge and its place in the gallery.
type EntryPage struct {
	Entry     Entry     `json:"entry"`
	Challenge Challenge `json:"challenge"`
	Place     int       `json:"place"` // 0 while not in the gallery (not scored yet)
	Of        int       `json:"of"`
	Prev      *string   `json:"prev"` // neighbours in gallery order
	Next      *string   `json:"next"`
}

func notFound() error { return httpx.NotFound() }

func invalid(msg string) error {
	return httpx.New(http.StatusUnprocessableEntity, "invalid_upload", msg)
}

// entryCols expects aliases e (build_entries), u (users) and $1 = the viewer's user id (” when anonymous).
const entryCols = `e.id, e.challenge, u.handle, e.made_with, e.status, e.score, e.checks, e.failure_reason, e.log_tail, e.shot IS NOT NULL, e.uploads,
	(SELECT count(*) FROM build_votes v WHERE v.entry_id = e.id),
	EXISTS (SELECT 1 FROM build_votes v WHERE v.entry_id = e.id AND v.user_id = $1), e.user_id = $1, e.created_at, e.updated_at`

// galleryWhere and galleryOrder define the public gallery of challenge $2.
const (
	galleryWhere = `e.challenge = $2 AND e.status = 'done' AND e.score IS NOT NULL AND u.banned_at IS NULL`
	galleryOrder = `(SELECT count(*) FROM build_votes v WHERE v.entry_id = e.id) DESC, e.score DESC, e.finished_at, e.id`
)

type scanner interface{ Scan(...any) error }

func scanEntry(row scanner) (Entry, error) {
	var e Entry
	var checks []byte
	if err := row.Scan(&e.ID, &e.Challenge, &e.Handle, &e.MadeWith, &e.Status, &e.Score, &checks, &e.FailureReason, &e.LogTail, &e.HasShot, &e.Version,
		&e.Votes, &e.Voted, &e.Mine, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return Entry{}, err
	}
	e.CreatedAt, e.UpdatedAt = e.CreatedAt.UTC(), e.UpdatedAt.UTC()
	e.Checks = checks
	if !e.Mine {
		e.LogTail = ""
	}
	return e, nil
}

func (s *Service) gallery(ctx context.Context, tx pgx.Tx, slug, viewer string, limit int) ([]Entry, error) {
	rows, err := tx.Query(ctx, `SELECT `+entryCols+` FROM build_entries e JOIN users u ON u.id = e.user_id
		WHERE `+galleryWhere+` ORDER BY `+galleryOrder+` LIMIT $3`, viewer, slug, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Service) mine(ctx context.Context, tx pgx.Tx, slug, viewer string) (*Entry, error) {
	if viewer == "" {
		return nil, nil
	}
	e, err := scanEntry(tx.QueryRow(ctx, `SELECT `+entryCols+` FROM build_entries e JOIN users u ON u.id = e.user_id
		WHERE e.challenge = $2 AND e.user_id = $1`, viewer, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// List is the home page: every open challenge with its numbers and best entries.
func (s *Service) List(ctx context.Context, viewer string) ([]Card, error) {
	cs := open(s.now().UTC())
	out := make([]Card, 0, len(cs))
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, c := range cs {
			card := Card{Challenge: *c}
			if err := tx.QueryRow(ctx, `SELECT count(*), coalesce(sum((SELECT count(*) FROM build_votes v WHERE v.entry_id = e.id)), 0)
				FROM build_entries e JOIN users u ON u.id = e.user_id WHERE `+strings.ReplaceAll(galleryWhere, "$2", "$1"), c.Slug).Scan(&card.Entries, &card.Votes); err != nil {
				return err
			}
			var err error
			if card.Top, err = s.gallery(ctx, tx, c.Slug, viewer, 3); err != nil {
				return err
			}
			if card.Mine, err = s.mine(ctx, tx, c.Slug, viewer); err != nil {
				return err
			}
			out = append(out, card)
		}
		return nil
	})
	return out, err
}

// Page is everything a challenge's page shows.
func (s *Service) Page(ctx context.Context, slug, viewer string) (Page, error) {
	c := find(slug, s.now().UTC())
	if c == nil {
		return Page{}, notFound()
	}
	p := Page{Challenge: ChallengeView{Challenge: *c, TaskMD: c.TaskMD, ScenarioCount: len(c.Scenarios)}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		if p.Entries, err = s.gallery(ctx, tx, c.Slug, viewer, galleryLimit); err != nil {
			return err
		}
		p.Challenge.Entries = len(p.Entries)
		p.Mine, err = s.mine(ctx, tx, c.Slug, viewer)
		return err
	})
	return p, err
}

// EntryPage is one solution: public once scored, its owner sees it from the upload on.
func (s *Service) EntryPage(ctx context.Context, id, viewer string) (EntryPage, error) {
	var p EntryPage
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		e, err := scanEntry(tx.QueryRow(ctx, `SELECT `+entryCols+` FROM build_entries e JOIN users u ON u.id = e.user_id
			WHERE e.id = $2 AND u.banned_at IS NULL`, viewer, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		}
		if err != nil {
			return err
		}
		if !e.Mine && (e.Status != "done" || e.Score == nil) {
			return notFound()
		}
		p.Entry = e
		var ids []string
		rows, err := tx.Query(ctx, `SELECT e.id FROM build_entries e JOIN users u ON u.id = e.user_id
			WHERE `+strings.ReplaceAll(galleryWhere, "$2", "$1")+` ORDER BY `+galleryOrder, e.Challenge)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var x string
			if err := rows.Scan(&x); err != nil {
				return err
			}
			ids = append(ids, x)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		p.Of = len(ids)
		for i, x := range ids {
			if x != id {
				continue
			}
			p.Place = i + 1
			if i > 0 {
				p.Prev = &ids[i-1]
			}
			if i+1 < len(ids) {
				p.Next = &ids[i+1]
			}
		}
		return nil
	})
	if err != nil {
		return EntryPage{}, err
	}
	for i := range catalog {
		if catalog[i].Slug == p.Entry.Challenge {
			p.Challenge = catalog[i]
		}
	}
	return p, nil
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
		"up to 5 MB, static files only, no network. Upload it at " + strings.TrimRight(publicURL, "/") + "/c/" + c.Slug + "\n"
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
