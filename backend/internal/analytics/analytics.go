// Package analytics is first-party product analytics: an events table filled by a small browser beacon
// (page views by route pattern, key UI actions) and by one-line Track calls where actions really happen.
// No third-party trackers, no cookies, no IPs; the admin funnel reads it back.
package analytics

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
)

// Retention is how long raw events are kept.
const Retention = 90 * 24 * time.Hour

type Event struct {
	Name   string
	UserID string
	AnonID string
	Path   string
	Ref    string
	UTM    string
	Entry  bool
	Props  map[string]any
}

type Service struct {
	pool *db.Pool
	ch   chan Event
}

func NewService(pool *db.Pool) *Service { return &Service{pool: pool, ch: make(chan Event, 1024)} }

var def atomic.Pointer[Service]

// Use makes s the target of Track and Prune. Packages that never call it (tests) track nothing.
func Use(s *Service) { def.Store(s) }

// Track records a server-side action. It never blocks and never fails the caller: when the buffer is full
// the event is dropped.
func Track(name, userID string, props map[string]any) {
	s := def.Load()
	if s == nil {
		return
	}
	select {
	case s.ch <- Event{Name: name, UserID: userID, Props: props}:
	default:
	}
}

// Prune deletes raw events past the retention window.
func Prune(ctx context.Context) (int64, error) {
	s := def.Load()
	if s == nil {
		return 0, nil
	}
	var n int64
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM events WHERE at < now() - $1::interval`, Retention.String())
		n = tag.RowsAffected()
		return err
	})
	return n, err
}

// Run writes tracked events in small batches until ctx is done.
func (s *Service) Run(ctx context.Context, log *slog.Logger) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	var buf []Event
	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := s.Record(context.WithoutCancel(ctx), buf); err != nil {
			log.Error("analytics: record", "err", err)
		}
		buf = buf[:0]
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case e := <-s.ch:
			buf = append(buf, e)
			if len(buf) >= 100 {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}

// Record inserts events in one statement.
func (s *Service) Record(ctx context.Context, evs []Event) error {
	if len(evs) == 0 {
		return nil
	}
	n := len(evs)
	name, user, anon, path, ref, utm := make([]string, n), make([]*string, n), make([]*string, n), make([]*string, n), make([]*string, n), make([]*string, n)
	entry, props := make([]bool, n), make([]*string, n)
	opt := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	for i, e := range evs {
		name[i], user[i], anon[i], path[i], ref[i], utm[i], entry[i] = e.Name, opt(e.UserID), opt(e.AnonID), opt(e.Path), opt(e.Ref), opt(e.UTM), e.Entry
		if len(e.Props) > 0 {
			if b, err := json.Marshal(e.Props); err == nil {
				props[i] = opt(string(b))
			}
		}
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO events (name, user_id, anon_id, path, ref, utm, entry, props)
			SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::bool[], $8::text[]::jsonb[])`,
			name, user, anon, path, ref, utm, entry, props)
		return err
	})
}

var (
	anonRe = regexp.MustCompile(`^[a-f0-9]{16,64}$`)
	nameRe = regexp.MustCompile(`^ui\.[a-z0-9_]{1,40}$`)
	hostRe = regexp.MustCompile(`^[a-z0-9.-]{1,100}$`)
	pathRe = regexp.MustCompile(`^/[A-Za-z0-9_\-./\[\]]{0,100}$`)
	srcRe  = regexp.MustCompile(`^[a-z0-9_.\- ]{1,40}$`)
)

// clientNames are the only non-"ui." events the browser may send.
var clientNames = map[string]bool{"page_view": true, "daily.download": true}

// Clean validates an event from the browser; ok is false when it should be dropped.
func Clean(e Event) (Event, bool) {
	if !clientNames[e.Name] && !nameRe.MatchString(e.Name) {
		return e, false
	}
	if e.Path != "" && !pathRe.MatchString(e.Path) {
		return e, false
	}
	e.Ref = strings.ToLower(e.Ref)
	if e.Ref != "" && !hostRe.MatchString(e.Ref) {
		e.Ref = ""
	}
	e.UTM = strings.ToLower(strings.TrimSpace(e.UTM))
	if e.UTM != "" && !srcRe.MatchString(e.UTM) {
		e.UTM = ""
	}
	e.Props = nil
	if e.Name != "page_view" {
		e.Ref, e.UTM, e.Entry = "", "", false
	}
	return e, true
}
