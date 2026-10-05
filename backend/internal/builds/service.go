package builds

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
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
	// galleryLimit caps what a gallery reads; a challenge is one screen of cards, not an archive.
	galleryLimit = 500

	// TestPoints and VotePoints make up the final score.
	TestPoints = 60
	VotePoints = 40
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

// ChallengeView is a challenge as the site shows it. Contract is left out until the challenge opens; the
// hidden tests are only ever counted.
type ChallengeView struct {
	Challenge
	Status   string `json:"status"`
	Tests    int    `json:"tests"`
	Entries  int    `json:"entries"`
	Votes    int    `json:"votes"`
	Contract *Text  `json:"contract,omitempty"`
}

// Card is a challenge on the home page with its best entries.
type Card struct {
	ChallengeView
	Top  []Entry `json:"top"` // up to 3 in ranking order, the reference last when there are fewer
	Mine *Entry  `json:"mine"`
}

// Entry is one person's upload. TestScore (0..60) comes from the hidden tests, VoteScore (0..40) from votes
// relative to the most-voted entry of the challenge; Score is their sum, nil until the tests have run.
type Entry struct {
	ID            string          `json:"id"`
	Challenge     string          `json:"challenge"`
	Handle        string          `json:"handle"`
	MadeWith      string          `json:"made_with"`
	Status        string          `json:"status"`
	TestScore     *int            `json:"test_score"`
	VoteScore     int             `json:"vote_score"`
	Score         *int            `json:"score"`
	Place         int             `json:"place"` // 1-based among ranked entries; 0 for the reference or while unscored
	Checks        json.RawMessage `json:"checks"`
	FailureReason *string         `json:"failure_reason"`
	LogTail       string          `json:"log_tail,omitempty"` // the owner only
	HasShot       bool            `json:"has_shot"`
	Version       int             `json:"version"` // bumps on every re-upload (cache key for the screenshot)
	Votes         int             `json:"votes"`
	Voted         bool            `json:"voted"`
	Mine          bool            `json:"mine"`
	Reference     bool            `json:"reference"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`

	finishedAt *time.Time
}

type Page struct {
	Challenge ChallengeView `json:"challenge"`
	Entries   []Entry       `json:"entries"`   // ranked
	Reference *Entry        `json:"reference"` // the platform's reference solution, out of ranking
	Mine      *Entry        `json:"mine"`
}

// EntryPage is one solution with its challenge and its neighbours in the ranking.
type EntryPage struct {
	Entry     Entry         `json:"entry"`
	Challenge ChallengeView `json:"challenge"`
	Of        int           `json:"of"`
	Prev      *string       `json:"prev"`
	Next      *string       `json:"next"`
}

func notFound() error { return httpx.NotFound() }

func invalid(msg string) error {
	return httpx.New(http.StatusUnprocessableEntity, "invalid_upload", msg)
}

// entryCols expects aliases e (build_entries), u (users) and $1 = the viewer's user id (” when anonymous).
const entryCols = `e.id, e.challenge, u.handle, e.made_with, e.status, e.score, e.checks, e.failure_reason, e.log_tail, e.shot IS NOT NULL, e.uploads,
	(SELECT count(*) FROM build_votes v WHERE v.entry_id = e.id),
	EXISTS (SELECT 1 FROM build_votes v WHERE v.entry_id = e.id AND v.user_id = $1), e.user_id = $1, e.reference, e.created_at, e.updated_at, e.finished_at`

type scanner interface{ Scan(...any) error }

func scanEntry(row scanner) (Entry, error) {
	var e Entry
	var checks []byte
	if err := row.Scan(&e.ID, &e.Challenge, &e.Handle, &e.MadeWith, &e.Status, &e.TestScore, &checks, &e.FailureReason, &e.LogTail, &e.HasShot,
		&e.Version, &e.Votes, &e.Voted, &e.Mine, &e.Reference, &e.CreatedAt, &e.UpdatedAt, &e.finishedAt); err != nil {
		return Entry{}, err
	}
	e.CreatedAt, e.UpdatedAt = e.CreatedAt.UTC(), e.UpdatedAt.UTC()
	e.Checks = checks
	if !e.Mine {
		e.LogTail = ""
	}
	if e.TestScore != nil {
		v := *e.TestScore
		e.Score = &v
	}
	return e, nil
}

// conceal keeps the hidden tests hidden while the challenge is open: only the owner sees which of their tests
// failed (by name); everyone sees how many passed.
func conceal(e *Entry, c *Challenge, now time.Time) {
	if e.Mine || c.Status(now) == StatusClosed || len(e.Checks) == 0 {
		return
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(e.Checks, &m) != nil {
		return
	}
	delete(m, "failed")
	if b, err := json.Marshal(m); err == nil {
		e.Checks = b
	}
}

// rank fills vote points, final scores and places of a challenge's scored entries and sorts them: final score,
// then test score, then votes, then who finished first. The reference takes no part.
func rank(es []Entry) {
	maxVotes := 0
	for _, e := range es {
		if !e.Reference {
			maxVotes = max(maxVotes, e.Votes)
		}
	}
	for i := range es {
		e := &es[i]
		if e.Reference || e.TestScore == nil {
			continue
		}
		if maxVotes > 0 {
			e.VoteScore = (2*VotePoints*e.Votes + maxVotes) / (2 * maxVotes) // rounded half up
		}
		v := *e.TestScore + e.VoteScore
		e.Score = &v
	}
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if a.Reference != b.Reference {
			return b.Reference
		}
		as, bs := score0(a.Score), score0(b.Score)
		if as != bs {
			return as > bs
		}
		if score0(a.TestScore) != score0(b.TestScore) {
			return score0(a.TestScore) > score0(b.TestScore)
		}
		if a.Votes != b.Votes {
			return a.Votes > b.Votes
		}
		if a.finishedAt != nil && b.finishedAt != nil && !a.finishedAt.Equal(*b.finishedAt) {
			return a.finishedAt.Before(*b.finishedAt)
		}
		return a.ID < b.ID
	})
	place := 0
	for i := range es {
		if !es[i].Reference && es[i].Score != nil {
			place++
			es[i].Place = place
		}
	}
}

func score0(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

// gallery reads a challenge's scored entries, ranked, with the reference last.
func (s *Service) gallery(ctx context.Context, tx pgx.Tx, c *Challenge, viewer string) ([]Entry, error) {
	rows, err := tx.Query(ctx, `SELECT `+entryCols+` FROM build_entries e JOIN users u ON u.id = e.user_id
		WHERE e.challenge = $2 AND e.status = 'done' AND e.score IS NOT NULL AND u.banned_at IS NULL
		ORDER BY e.finished_at LIMIT $3`, viewer, c.Slug, galleryLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	now := s.now().UTC()
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		conceal(&e, c, now)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rank(out)
	return out, nil
}

func (s *Service) mine(ctx context.Context, tx pgx.Tx, c *Challenge, viewer string, ranked []Entry) (*Entry, error) {
	if viewer == "" {
		return nil, nil
	}
	for i := range ranked {
		if ranked[i].Mine && !ranked[i].Reference {
			e := ranked[i]
			return &e, nil
		}
	}
	e, err := scanEntry(tx.QueryRow(ctx, `SELECT `+entryCols+` FROM build_entries e JOIN users u ON u.id = e.user_id
		WHERE e.challenge = $2 AND e.user_id = $1 AND NOT e.reference`, viewer, c.Slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Service) view(c *Challenge, ranked []Entry, withContract bool) ChallengeView {
	v := ChallengeView{Challenge: *c, Status: c.Status(s.now().UTC()), Tests: len(c.Scenarios)}
	for _, e := range ranked {
		if !e.Reference {
			v.Entries++
			v.Votes += e.Votes
		}
	}
	if withContract && v.Status != StatusUpcoming {
		t := c.Contract
		v.Contract = &t
	}
	return v
}

// List is the home page: the season's challenges in order.
func (s *Service) List(ctx context.Context, viewer string) ([]Card, error) {
	var out []Card
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, c := range visible() {
			if c.Status(s.now().UTC()) == StatusUpcoming {
				out = append(out, Card{ChallengeView: s.view(c, nil, false), Top: []Entry{}})
				continue
			}
			ranked, err := s.gallery(ctx, tx, c, viewer)
			if err != nil {
				return err
			}
			card := Card{ChallengeView: s.view(c, ranked, false), Top: []Entry{}}
			for _, e := range ranked {
				if len(card.Top) < 3 {
					card.Top = append(card.Top, e)
				}
			}
			if card.Mine, err = s.mine(ctx, tx, c, viewer, ranked); err != nil {
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
	c := find(slug)
	if c == nil {
		return Page{}, notFound()
	}
	p := Page{Entries: []Entry{}}
	if c.Status(s.now().UTC()) == StatusUpcoming {
		p.Challenge = s.view(c, nil, true)
		return p, nil
	}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		ranked, err := s.gallery(ctx, tx, c, viewer)
		if err != nil {
			return err
		}
		for _, e := range ranked {
			if e.Reference {
				e := e
				p.Reference = &e
			} else {
				p.Entries = append(p.Entries, e)
			}
		}
		p.Challenge = s.view(c, ranked, true)
		p.Mine, err = s.mine(ctx, tx, c, viewer, ranked)
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
		c := find(e.Challenge)
		if c == nil || (!e.Mine && (e.Status != "done" || e.TestScore == nil)) {
			return notFound()
		}
		ranked, err := s.gallery(ctx, tx, c, viewer)
		if err != nil {
			return err
		}
		p.Challenge = s.view(c, ranked, false)
		var ids []string
		for _, r := range ranked {
			if !r.Reference {
				ids = append(ids, r.ID)
			}
			if r.ID == id {
				e = r // ranked: with vote points, final score and place
			}
		}
		conceal(&e, c, s.now().UTC())
		p.Entry, p.Of = e, len(ids)
		for i, x := range ids {
			if x != id {
				continue
			}
			if i > 0 {
				p.Prev = &ids[i-1]
			}
			if i+1 < len(ids) {
				p.Next = &ids[i+1]
			}
		}
		return nil
	})
	return p, err
}

// digest identifies an upload by its content, whatever zip tool packed it.
func digest(files map[string][]byte) string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		sum := sha256.Sum256(files[p])
		fmt.Fprintf(h, "%s\x00%x\n", p, sum)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Create stores an upload as the person's entry for an open challenge (replacing their previous one) and
// queues it. A single .html file is accepted as the site's index.html; a copy of another entry is refused.
func (s *Service) Create(ctx context.Context, userID, slug, filename string, data []byte, madeWith string) (Entry, error) {
	c := find(slug)
	if c == nil {
		return Entry{}, notFound()
	}
	switch c.Status(s.now().UTC()) {
	case StatusUpcoming:
		return Entry{}, httpx.StateConflict("This challenge has not opened yet")
	case StatusClosed:
		return Entry{}, httpx.StateConflict("This challenge is closed")
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
	sum := digest(files)
	madeWith = sanitize.CleanText(madeWith, maxMadeWith)
	var id string
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var copied bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM build_entries WHERE challenge = $1 AND digest = $2 AND user_id <> $3)`,
			c.Slug, sum, userID).Scan(&copied); err != nil {
			return err
		}
		if copied {
			return httpx.StateConflict("This solution is identical to another entry")
		}
		return tx.QueryRow(ctx, `
			INSERT INTO build_entries (id, challenge, user_id, zip, made_with, digest) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (challenge, user_id) DO UPDATE SET zip = EXCLUDED.zip, made_with = EXCLUDED.made_with, digest = EXCLUDED.digest,
				status = 'queued', score = NULL, checks = '{}'::jsonb, shot = NULL, log_tail = '', failure_reason = NULL,
				uploads = build_entries.uploads + 1, updated_at = now(), finished_at = NULL
			RETURNING id`, idgen.New("bld"), c.Slug, userID, data, madeWith, sum).Scan(&id)
	})
	if err != nil {
		return Entry{}, err
	}
	return s.entry(ctx, id, userID)
}

func singleFileZip(name string, body []byte) ([]byte, error) {
	return zipFiles(map[string][]byte{name: body})
}

func zipFiles(files map[string][]byte) ([]byte, error) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range paths {
		w, err := zw.Create(p)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[p]); err != nil {
			return nil, err
		}
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
	if c := bySlug(e.Challenge); err == nil && c != nil {
		conceal(&e, c, s.now().UTC())
	}
	return e, err
}

// Vote adds (on) or removes the viewer's vote for a scored entry of an open challenge that is not theirs.
func (s *Service) Vote(ctx context.Context, userID, entryID string, on bool) (Entry, error) {
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var owner, status, slug string
		var ref bool
		err := tx.QueryRow(ctx, `SELECT user_id, status, challenge, reference FROM build_entries WHERE id = $1`, entryID).Scan(&owner, &status, &slug, &ref)
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		}
		if err != nil {
			return err
		}
		c := find(slug)
		switch {
		case c == nil:
			return notFound()
		case c.Status(s.now().UTC()) != StatusOpen:
			return httpx.StateConflict("Voting is closed")
		case ref:
			return httpx.StateConflict("The reference solution is not voted on")
		case owner == userID:
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

// Delete removes an entry: its owner, or an admin (spam). The reference is the admin's alone.
func (s *Service) Delete(ctx context.Context, userID string, admin bool, entryID string) error {
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM build_entries WHERE id = $1 AND ((user_id = $2 AND NOT reference) OR $3)`, entryID, userID, admin)
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

// CommonRules is what every challenge's contract starts with (the page shows it translated).
const CommonRules = `## Rules for every challenge

- Upload a zip with index.html at its root (or a single .html file). Every resource is local: no network
  requests, no CDNs, no web fonts.
- The window.* API controls the very same state the interface shows.
- Final score: 60% hidden tests + 40% votes.
`

// TaskZip is the download: <slug>/CONTRACT.md (English), CONTRACT.ru.md and a README on how to hand it in.
func (s *Service) TaskZip(slug, publicURL string) ([]byte, error) {
	c := find(slug)
	if c == nil || c.Status(s.now().UTC()) == StatusUpcoming {
		return nil, notFound()
	}
	if publicURL == "" {
		publicURL = "https://tolerance.cc"
	}
	readme := "# " + c.Title.En + "\n\n" + c.OneLiner.En + "\n\n" +
		"Give CONTRACT.md to your coding agent and let it build the page.\n\n" + CommonRules + "\n" +
		"Upload it at " + strings.TrimRight(publicURL, "/") + "/c/" + c.Slug + "\n"
	files := map[string][]byte{c.Slug + "/README.md": []byte(readme), c.Slug + "/CONTRACT.md": []byte(c.Contract.En)}
	if c.Contract.Ru != "" {
		files[c.Slug+"/CONTRACT.ru.md"] = []byte(c.Contract.Ru)
	}
	return zipFiles(files)
}
