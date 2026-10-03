package products

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/platform/limits"
	"tolerance/internal/platform/sanitize"
	"tolerance/internal/submissions"
)

type Service struct{ pool *db.Pool }

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

func invalid(msg string) error {
	return httpx.WithField(http.StatusUnprocessableEntity, "invalid_upload", msg, "file", "invalid")
}

func phaseOf(deadline time.Time) string {
	now := time.Now()
	switch {
	case now.Before(deadline):
		return PhaseOpen
	case now.Before(deadline.Add(VotingWindow)):
		return PhaseVoting
	}
	return PhaseFinal
}

// published reports whether entries are visible to everyone (the deadline has passed).
func published(deadline time.Time) bool { return phaseOf(deadline) != PhaseOpen }

const taskCols = `t.slug, t.title, t.summary, t.kind, t.opens_at, t.deadline, jsonb_array_length(t.scenarios), t.bench IS NOT NULL,
	(SELECT count(DISTINCT e.user_id) FROM product_entries e WHERE e.task_slug = t.slug AND e.status = 'done' AND e.hidden_at IS NULL)`

func scanTask(row scanner) (Task, error) {
	var t Task
	if err := row.Scan(&t.Slug, &t.Title, &t.Summary, &t.Kind, &t.OpensAt, &t.Deadline, &t.ScenarioCount, &t.HasBench, &t.EntryCount); err != nil {
		return Task{}, err
	}
	t.OpensAt, t.Deadline = t.OpensAt.UTC(), t.Deadline.UTC()
	t.Phase = phaseOf(t.Deadline)
	t.VotingEndsAt = t.Deadline.Add(VotingWindow)
	t.Attempts = limits.Cap(AttemptsPerTask)
	return t, nil
}

// Listing is the public list: every task that has opened (this week's, last week's in voting, the archive),
// newest first, plus what is known about the tasks still to come.
type Listing struct {
	Items    []Task   `json:"items"`
	Upcoming Upcoming `json:"upcoming"`
}

func (s *Service) List(ctx context.Context) (Listing, error) {
	if err := s.EnsureWeek(ctx); err != nil {
		return Listing{}, err
	}
	out := Listing{Items: []Task{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+taskCols+` FROM product_tasks t WHERE t.active AND t.opens_at <= now() ORDER BY t.opens_at DESC, t.slug`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t, err := scanTask(rows)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, t)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		for i, t := range out.Items {
			if t.Phase == PhaseFinal {
				if out.Items[i].Winner, err = winnerOf(ctx, tx, t.Slug, t.Kind); err != nil {
					return err
				}
			}
		}
		out.Upcoming, err = upcoming(ctx, tx)
		return err
	})
	return out, err
}

// Detail is a task with the viewer's own entries (newest first); Mine is empty for anonymous viewers.
type Detail struct {
	Task
	AttemptsUsed int     `json:"attempts_used"`
	Mine         []Entry `json:"mine"`
	// FastestMS is the best benchmark time among the counted entries; only known once the deadline has passed (entries are hidden before).
	FastestMS *float64 `json:"fastest_ms"`
}

func (s *Service) Get(ctx context.Context, slug, userID string) (Detail, error) {
	d := Detail{Mine: []Entry{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		d.Task, err = scanTask(tx.QueryRow(ctx, `SELECT `+taskCols+` FROM product_tasks t WHERE t.slug = $1 AND t.active AND t.opens_at <= now()`, slug))
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT task_md FROM product_tasks WHERE slug = $1`, slug).Scan(&d.TaskMD); err != nil {
			return err
		}
		if d.Phase != PhaseOpen && d.Kind == KindCLI {
			pick, _ := rankRule(KindCLI)
			if err := tx.QueryRow(ctx, `SELECT min(bench_ms) FROM (SELECT DISTINCT ON (user_id) bench_ms FROM product_entries WHERE task_slug = $1 AND status = 'done' AND hidden_at IS NULL ORDER BY user_id, `+pick+`) c`, slug).Scan(&d.FastestMS); err != nil {
				return err
			}
		}
		if userID == "" {
			return nil
		}
		rows, err := tx.Query(ctx, `SELECT `+entryCols+` FROM product_entries e JOIN users u ON u.id = e.user_id
			WHERE e.task_slug = $2 AND e.user_id = $1 ORDER BY e.created_at DESC`, userID, slug)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			e, err := scanEntry(rows)
			if err != nil {
				return err
			}
			d.Mine = append(d.Mine, e)
			if e.Status != StatusInfraError {
				d.AttemptsUsed++
			}
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, httpx.NotFound()
	}
	return d, err
}

// Create validates a zip upload, stores it as a queued entry and enqueues its scoring run.
func (s *Service) Create(ctx context.Context, userID, slug, filename string, data []byte, madeWith string) (Entry, error) {
	if strings.ToLower(filepath.Ext(filename)) != ".zip" {
		return Entry{}, invalid("Upload a .zip of your project (files at the root)")
	}
	files, err := submissions.ReadZip(data)
	if err != nil {
		return Entry{}, err
	}
	if len(files) == 0 {
		return Entry{}, invalid("The zip has no files")
	}
	madeWith = sanitize.CleanText(madeWith, maxMadeWith)

	id := idgen.New("pe")
	var out Entry
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Serialize a person's uploads so two parallel requests cannot both take the last attempt.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('products:' || $1))`, userID); err != nil {
			return err
		}
		var deadline time.Time
		var total int
		var kind string
		err := tx.QueryRow(ctx, `SELECT deadline, jsonb_array_length(scenarios), kind FROM product_tasks WHERE slug = $1 AND active AND opens_at <= now()`, slug).
			Scan(&deadline, &total, &kind)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		if published(deadline) {
			return httpx.New(http.StatusConflict, "deadline_passed", "The deadline has passed; uploads are closed")
		}
		var used int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM product_entries WHERE task_slug = $1 AND user_id = $2 AND status <> 'infra_error'`,
			slug, userID).Scan(&used); err != nil {
			return err
		}
		if used >= limits.Cap(AttemptsPerTask) {
			return httpx.New(http.StatusTooManyRequests, "attempts_exhausted", "All attempts for this task are used")
		}
		if kind == KindSite && total == 0 {
			// Nothing to run: a site is done once its zip holds an index.html, and votes decide.
			if _, ok := files["index.html"]; !ok {
				return invalid("The zip needs an index.html at its root (or inside a single top-level folder)")
			}
			if _, err := tx.Exec(ctx, `INSERT INTO product_entries (id, task_slug, user_id, zip, made_with, status, finished_at)
				VALUES ($1,$2,$3,$4,$5,'done',now())`, id, slug, userID, data, madeWith); err != nil {
				return err
			}
		} else {
			if kind == KindSite {
				if _, ok := files["index.html"]; !ok {
					return invalid("The zip needs an index.html at its root (or inside a single top-level folder)")
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO product_entries (id, task_slug, user_id, zip, made_with, total) VALUES ($1,$2,$3,$4,$5,$6)`,
				id, slug, userID, data, madeWith, total); err != nil {
				return err
			}
			if _, err := jobs.Enqueue(ctx, tx, JobKind, RunPayload{EntryID: id}, ""); err != nil {
				return err
			}
		}
		if err := audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "product_entry.created", AggregateKind: "product_entry", AggregateID: id,
			RequestID: httpx.RequestID(ctx)}); err != nil {
			return err
		}
		out, err = scanEntry(tx.QueryRow(ctx, `SELECT `+entryCols+` FROM product_entries e JOIN users u ON u.id = e.user_id WHERE e.id = $2`, userID, id))
		return err
	})
	return out, err
}

// Zip returns a published entry's upload (after the deadline only).
func (s *Service) Zip(ctx context.Context, entryID string) ([]byte, error) {
	var data []byte
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var deadline time.Time
		var kind string
		if err := tx.QueryRow(ctx, `SELECT e.zip, t.deadline, t.kind FROM product_entries e JOIN product_tasks t ON t.slug = e.task_slug
			WHERE e.id = $1 AND e.status = 'done' AND e.hidden_at IS NULL`, entryID).Scan(&data, &deadline, &kind); err != nil {
			return err
		}
		if !published(deadline) {
			return httpx.NotFound()
		}
		// Sites are judged blind while voting is open; their source would give the author away.
		if kind == KindSite && phaseOf(deadline) == PhaseVoting {
			return httpx.Forbidden("Source and downloads of sites open when voting ends")
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.NotFound()
	}
	return data, err
}

// SiteFiles returns a site entry's files when the viewer may see them: its owner any time, everyone else
// once the deadline has passed.
func (s *Service) SiteFiles(ctx context.Context, entryID, viewerID string) (map[string][]byte, error) {
	var data []byte
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var deadline time.Time
		var owner string
		if err := tx.QueryRow(ctx, `SELECT e.zip, e.user_id, t.deadline FROM product_entries e JOIN product_tasks t ON t.slug = e.task_slug
			WHERE e.id = $1 AND e.status = 'done' AND t.kind = 'site' AND (e.hidden_at IS NULL OR e.user_id = $2)`, entryID, viewerID).Scan(&data, &owner, &deadline); err != nil {
			return err
		}
		if owner != viewerID && !published(deadline) {
			return httpx.NotFound()
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.NotFound()
	}
	if err != nil {
		return nil, err
	}
	return submissions.ReadZip(data)
}
