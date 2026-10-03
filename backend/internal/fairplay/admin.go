package fairplay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/limits"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/platform/sanitize"
)

var reasons = map[string]bool{"cheating": true, "copied": true, "multi_account": true, "offensive": true, "other": true}

const maxDetails = 500

// Report files a report from a signed-in user about a person (target = handle). A person can report the same target once until it is reviewed.
func (s *Service) Report(ctx context.Context, reporterID, kind, target, reason, details string) error {
	if kind != "user" {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "target_kind must be user", "target_kind", "invalid")
	}
	if !reasons[reason] {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "Pick a reason", "reason", "invalid")
	}
	details = sanitize.CleanText(details, maxDetails)
	if reason == "other" && details == "" {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "Describe the problem", "details", "required")
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var id, owner string
		var err error
		// House agents are trusted; to_jsonb keeps this valid whether or not users.house exists yet.
		err = tx.QueryRow(ctx, `SELECT id, id FROM users u WHERE lower(handle) = lower($1) AND NOT coalesce((to_jsonb(u)->>'house')::boolean, false)`, target).Scan(&id, &owner)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		if owner == reporterID {
			return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "You cannot report yourself", "target", "invalid")
		}
		tag, err := tx.Exec(ctx, `INSERT INTO fairplay_reports (id, reporter_id, target_kind, target_id, reason, details) VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT DO NOTHING`, idgen.New("fr"), reporterID, kind, id, reason, details)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.New(http.StatusConflict, "state_conflict", "You already reported this")
		}
		return nil
	})
}

type Flag struct {
	ID     string          `json:"id"`
	Signal string          `json:"signal"`
	Score  float64         `json:"score"`
	Detail json.RawMessage `json:"detail"`
}

// FlaggedItem is one upload with its open flags.
type FlaggedItem struct {
	SubjectKind  string    `json:"subject_kind"` // submission
	SubjectID    string    `json:"subject_id"`
	UserID       string    `json:"user_id"`
	Handle       string    `json:"handle"`
	Banned       bool      `json:"banned"`
	Hidden       bool      `json:"hidden"`
	TaskSlug     string    `json:"task_slug"`
	Day          *string   `json:"day"`
	SolveSeconds *int      `json:"solve_seconds"`
	At           time.Time `json:"at"`
	Score        float64   `json:"score"`
	Flags        []Flag    `json:"flags"`
}

type ReportItem struct {
	ID         string    `json:"id"`
	Reporter   string    `json:"reporter"`
	TargetKind string    `json:"target_kind"` // user
	TargetID   string    `json:"target_id"`
	UserID     string    `json:"user_id"` // the person reported
	Handle     string    `json:"handle"`
	Label      string    `json:"label"`
	Banned     bool      `json:"banned"`
	Hidden     bool      `json:"hidden"`
	Reason     string    `json:"reason"`
	Details    string    `json:"details"`
	At         time.Time `json:"at"`
	Others     int       `json:"others"` // other open reports on the same target
}

type ClusterUser struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Banned bool   `json:"banned"`
}

// Cluster is one hashed IP or device seen on several accounts.
type Cluster struct {
	Kind  string        `json:"kind"` // ip | device
	Hash  string        `json:"hash"` // shortened
	Users []ClusterUser `json:"users"`
}

type Overview struct {
	Flags    []FlaggedItem `json:"flags"`
	Reports  []ReportItem  `json:"reports"`
	Clusters []Cluster     `json:"clusters"`
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	out := Overview{Flags: []FlaggedItem{}, Reports: []ReportItem{}, Clusters: []Cluster{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT f.id, f.subject_kind, f.subject_id, f.user_id, u.handle, u.banned_at IS NOT NULL,
			       coalesce((SELECT s.hidden_at IS NOT NULL FROM submissions s WHERE s.id = f.subject_id), false),
			       coalesce(x.task_slug, ''), x.day::text, x.solve_seconds, f.created_at, f.signal, f.score, f.detail
			FROM fairplay_flags f JOIN users u ON u.id = f.user_id
			LEFT JOIN fairplay_uploads x ON x.subject_kind = f.subject_kind AND x.subject_id = f.subject_id
			WHERE f.status = 'open' AND NOT coalesce((to_jsonb(u)->>'house')::boolean, false) ORDER BY f.created_at DESC LIMIT 400`)
		if err != nil {
			return err
		}
		byKey := map[string]*FlaggedItem{}
		var order []string
		for rows.Next() {
			var it FlaggedItem
			var fl Flag
			if err := rows.Scan(&fl.ID, &it.SubjectKind, &it.SubjectID, &it.UserID, &it.Handle, &it.Banned, &it.Hidden,
				&it.TaskSlug, &it.Day, &it.SolveSeconds, &it.At, &fl.Signal, &fl.Score, &fl.Detail); err != nil {
				rows.Close()
				return err
			}
			key := it.SubjectKind + "/" + it.SubjectID
			cur := byKey[key]
			if cur == nil {
				it.At = it.At.UTC()
				cur = &it
				byKey[key] = cur
				order = append(order, key)
			}
			cur.Score += fl.Score
			cur.Flags = append(cur.Flags, fl)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, k := range order {
			out.Flags = append(out.Flags, *byKey[k])
		}
		sort.SliceStable(out.Flags, func(i, j int) bool { return out.Flags[i].Score > out.Flags[j].Score })

		rows, err = tx.Query(ctx, `
			SELECT r.id, rp.handle, r.target_kind, r.target_id, coalesce(t.id, ''), coalesce(t.handle, ''), '',
			       coalesce(t.banned_at IS NOT NULL, false), false, r.reason, r.details, r.created_at,
			       (SELECT count(*) FROM fairplay_reports o WHERE o.status = 'open' AND o.target_kind = r.target_kind AND o.target_id = r.target_id AND o.id <> r.id)::int
			FROM fairplay_reports r JOIN users rp ON rp.id = r.reporter_id
			LEFT JOIN users t ON t.id = r.target_id
			WHERE r.status = 'open' ORDER BY r.created_at DESC LIMIT 200`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var r ReportItem
			if err := rows.Scan(&r.ID, &r.Reporter, &r.TargetKind, &r.TargetID, &r.UserID, &r.Handle, &r.Label, &r.Banned, &r.Hidden, &r.Reason, &r.Details, &r.At, &r.Others); err != nil {
				rows.Close()
				return err
			}
			r.At = r.At.UTC()
			out.Reports = append(out.Reports, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		rows, err = tx.Query(ctx, `
			SELECT s.kind, substr(s.hash, 1, 8), jsonb_agg(jsonb_build_object('id', u.id, 'handle', u.handle, 'banned', u.banned_at IS NOT NULL) ORDER BY u.handle)
			FROM fairplay_seen s JOIN users u ON u.id = s.user_id
			WHERE (s.kind, s.hash) IN (SELECT kind, hash FROM fairplay_seen GROUP BY kind, hash HAVING count(*) > 1)
			GROUP BY s.kind, s.hash ORDER BY (s.kind = 'device') DESC, count(*) DESC, max(s.last_at) DESC LIMIT 50`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c Cluster
			var raw []byte
			if err := rows.Scan(&c.Kind, &c.Hash, &raw); err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &c.Users); err != nil {
				return err
			}
			out.Clusters = append(out.Clusters, c)
		}
		return rows.Err()
	})
	return out, err
}

func validStatus(st string) error {
	if st != "dismissed" && st != "actioned" {
		return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", "status must be dismissed or actioned", "status", "invalid")
	}
	return nil
}

// ResolveFlags closes every open flag of one upload.
func (s *Service) ResolveFlags(ctx context.Context, actorID, kind, id, status string) error {
	if err := validStatus(status); err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE fairplay_flags SET status = $4, reviewed_by = $1, reviewed_at = now()
			WHERE subject_kind = $2 AND subject_id = $3 AND status = 'open'`, actorID, kind, id, status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.New(http.StatusConflict, "state_conflict", "Nothing open to resolve")
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, Action: "fairplay.flags." + status, AggregateKind: kind, AggregateID: id,
			RequestID: httpx.RequestID(ctx)})
	})
}

// ResolveReports closes the open report with this id and every other open report on the same target.
func (s *Service) ResolveReport(ctx context.Context, actorID, id, status string) error {
	if err := validStatus(status); err != nil {
		return err
	}
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE fairplay_reports SET status = $3, reviewed_by = $2, reviewed_at = now()
			WHERE status = 'open' AND (target_kind, target_id) = (SELECT target_kind, target_id FROM fairplay_reports WHERE id = $1)`, id, actorID, status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NotFound()
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, Action: "fairplay.report." + status, AggregateKind: "report", AggregateID: id,
			RequestID: httpx.RequestID(ctx)})
	})
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	raw, err := httpx.ReadBody(w, r)
	if err == nil {
		err = httpx.Decode(raw, dst)
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return false
	}
	return true
}

func done(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.Respond(w, http.StatusOK, map[string]bool{"ok": true})
}

// RegisterOwnerRoutes mounts POST /reports (session required). Reporting is rate-limited per person.
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service, limiter *ratelimit.Limiter) {
	mux.HandleFunc("POST /api/v1/reports", func(w http.ResponseWriter, r *http.Request) {
		uid := identity.MustFromContext(r.Context()).UserID
		if !limiter.Allow("report:"+uid, limits.Cap(5), time.Hour) || !limiter.Allow("report-day:"+uid, limits.Cap(20), 24*time.Hour) {
			httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "You are reporting too fast; try again later"))
			return
		}
		var b struct {
			TargetKind string `json:"target_kind"`
			Target     string `json:"target"`
			Reason     string `json:"reason"`
			Details    string `json:"details"`
		}
		if readJSON(w, r, &b) {
			done(w, r, s.Report(r.Context(), uid, b.TargetKind, strings.TrimSpace(b.Target), b.Reason, b.Details))
		}
	})
}

// RegisterAdminRoutes mounts the fair-play queue; the caller wraps it in the session and admin guards.
func RegisterAdminRoutes(mux *http.ServeMux, s *Service) {
	actor := func(r *http.Request) string { return identity.MustFromContext(r.Context()).UserID }
	mux.HandleFunc("GET /api/v1/admin/fairplay", func(w http.ResponseWriter, r *http.Request) {
		o, err := s.Overview(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, o)
	})
	mux.HandleFunc("POST /api/v1/admin/fairplay/flags/resolve", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			SubjectKind string `json:"subject_kind"`
			SubjectID   string `json:"subject_id"`
			Status      string `json:"status"`
		}
		if readJSON(w, r, &b) {
			done(w, r, s.ResolveFlags(r.Context(), actor(r), b.SubjectKind, b.SubjectID, b.Status))
		}
	})
	mux.HandleFunc("POST /api/v1/admin/fairplay/reports/{id}/resolve", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Status string `json:"status"`
		}
		if readJSON(w, r, &b) {
			done(w, r, s.ResolveReport(r.Context(), actor(r), r.PathValue("id"), b.Status))
		}
	})
}
