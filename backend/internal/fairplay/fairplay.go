// Package fairplay records signals that help humans spot cheating on the daily task and the product of the week:
// time from first download to upload, accounts sharing a (hashed) IP or device, near-identical solutions, bursts of
// uploads. It also keeps the reports queue. Nothing here changes a verdict or punishes anyone; /admin shows the
// flags and the actions reuse internal/moderation.
package fairplay

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/clientip"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/idgen"
)

const (
	deviceCookie  = "arena_did"
	fastSolveSecs = 60
	burstCount    = 3
	burstWindow   = 10 * time.Minute
)

type Service struct {
	pool   *db.Pool
	secret []byte
}

// NewService hashes IPs and device ids with secret (ARENA_FAIRPLAY_SECRET). Without one a random key is used,
// so hashes only match within one process lifetime; set it to keep clusters across restarts.
func NewService(pool *db.Pool, secret string) *Service {
	key := []byte(secret)
	if len(key) == 0 {
		key = make([]byte, 32)
		_, _ = rand.Read(key)
		slog.Warn("ARENA_FAIRPLAY_SECRET is not set; fair-play IP/device hashes reset on restart")
	}
	return &Service{pool: pool, secret: key}
}

func (s *Service) hash(kind, v string) string {
	if v == "" {
		return ""
	}
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(kind + ":" + v))
	return hex.EncodeToString(m.Sum(nil))[:32]
}

type clientKey struct{}
type client struct{ ip, device string } // hashes

func clientOf(ctx context.Context) client {
	c, _ := ctx.Value(clientKey{}).(client)
	return c
}

// Client attaches the hashed IP and device id to every request's context and hands out the device cookie.
// The raw values are never stored. The device id is a random cookie, a hint (clearing it hides it), not proof.
func (s *Service) Client(next http.Handler, trustProxy, secure bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var device string
		if c, err := r.Cookie(deviceCookie); err == nil && len(c.Value) >= 16 && len(c.Value) <= 64 {
			device = c.Value
		} else {
			device = idgen.New("d")
			http.SetCookie(w, &http.Cookie{Name: deviceCookie, Value: device, Path: "/", MaxAge: 365 * 24 * 3600,
				HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure})
		}
		c := client{ip: s.hash("ip", clientip.FromRequest(r, trustProxy)), device: s.hash("device", device)}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientKey{}, c)))
	})
}

// RecordDownload wraps a public GET route: when the caller is signed in, remember the first time they fetched
// kind/{slug} (slug is the route's {slug}). The response is never affected.
func (s *Service) RecordDownload(kind string, who func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if uid := who(r); uid != "" {
				s.noteDownload(r.Context(), uid, kind, r.PathValue("slug"))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Service) noteDownload(ctx context.Context, userID, kind, slug string) {
	c := clientOf(ctx)
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO fairplay_downloads (user_id, kind, slug) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, userID, kind, slug); err != nil {
			return err
		}
		return noteSeen(ctx, tx, userID, c)
	})
	if err != nil {
		slog.Warn("fairplay: record download", "err", err)
	}
}

func noteSeen(ctx context.Context, tx pgx.Tx, userID string, c client) error {
	for kind, h := range map[string]string{"ip": c.ip, "device": c.device} {
		if h == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO fairplay_seen (user_id, kind, hash) VALUES ($1,$2,$3)
			ON CONFLICT (user_id, kind, hash) DO UPDATE SET last_at = now()`, userID, kind, h); err != nil {
			return err
		}
	}
	return nil
}

// Upload is one stored submission (kind "submission", content = its diff) that has just been accepted.
type Upload struct {
	Kind, ID, UserID, TaskSlug string
	Day                        *string
	Content                    string
}

// Observe records an upload's signals in the background. It never fails the upload: errors are only logged.
func (s *Service) Observe(ctx context.Context, u Upload) {
	c := clientOf(ctx)
	ctx = context.WithoutCancel(ctx)
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := s.observe(ctx, u, c); err != nil {
			slog.Warn("fairplay: observe upload", "subject", u.ID, "err", err)
		}
	}()
}

func (s *Service) observe(ctx context.Context, u Upload, c client) error {
	sketch := sketchOf(u.Kind, u.Content)
	dlKind := "task"
	return s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// One person's uploads are recorded one at a time, so burst counts and duplicate lookups are consistent.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('fairplay:' || $1))`, u.UserID); err != nil {
			return err
		}
		var solve *int
		if err := tx.QueryRow(ctx, `SELECT (extract(epoch FROM now() - first_at))::int FROM fairplay_downloads WHERE user_id = $1 AND kind = $2 AND slug = $3`,
			u.UserID, dlKind, u.TaskSlug).Scan(&solve); err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err := noteSeen(ctx, tx, u.UserID, c); err != nil {
			return err
		}
		if sketch == nil {
			sketch = []int64{}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO fairplay_uploads (subject_kind, subject_id, user_id, task_slug, day, ip_hash, device_hash, solve_seconds, sketch)
			VALUES ($1,$2,$3,$4,$5::date,$6,$7,$8,$9) ON CONFLICT DO NOTHING`,
			u.Kind, u.ID, u.UserID, u.TaskSlug, u.Day, c.ip, c.device, solve, sketch); err != nil {
			return err
		}
		f := flagger{tx: tx, kind: u.Kind}

		if solve != nil && *solve < fastSolveSecs {
			if err := f.raise(ctx, u.ID, u.UserID, "fast_solve", 1-float64(*solve)/fastSolveSecs, map[string]any{"seconds": *solve}); err != nil {
				return err
			}
		}
		var recent int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM fairplay_uploads WHERE user_id = $1 AND created_at > now() - $2::interval`,
			u.UserID, burstWindow.String()).Scan(&recent); err != nil {
			return err
		}
		if recent >= burstCount {
			if err := f.raise(ctx, u.ID, u.UserID, "burst", min(1, float64(recent)/(2*burstCount)),
				map[string]any{"uploads": recent, "minutes": int(burstWindow.Minutes())}); err != nil {
				return err
			}
		}
		if c.device != "" {
			others, err := handles(ctx, tx, `SELECT DISTINCT u.handle FROM fairplay_seen s JOIN users u ON u.id = s.user_id
				WHERE s.kind = 'device' AND s.hash = $1 AND s.user_id <> $2 LIMIT 10`, c.device, u.UserID)
			if err != nil {
				return err
			}
			if len(others) > 0 {
				if err := f.raise(ctx, u.ID, u.UserID, "shared_device", 0.8, map[string]any{"users": others}); err != nil {
					return err
				}
			}
		}
		if c.ip != "" {
			others, err := handles(ctx, tx, `SELECT DISTINCT us.handle FROM fairplay_uploads x JOIN users us ON us.id = x.user_id
				WHERE x.ip_hash = $1 AND x.user_id <> $2 AND x.subject_kind = $3 AND x.task_slug = $4 AND x.day IS NOT DISTINCT FROM $5::date LIMIT 10`,
				c.ip, u.UserID, u.Kind, u.TaskSlug, u.Day)
			if err != nil {
				return err
			}
			if len(others) > 0 {
				if err := f.raise(ctx, u.ID, u.UserID, "shared_ip", 0.4, map[string]any{"users": others}); err != nil {
					return err
				}
			}
		}
		return s.nearDuplicates(ctx, tx, f, u, sketch)
	})
}

// nearDuplicates compares the sketch with the latest upload of every other user for the same task and day,
// ignoring shingles that more than half of those users share (the minimal fix everyone makes).
func (s *Service) nearDuplicates(ctx context.Context, tx pgx.Tx, f flagger, u Upload, sketch []int64) error {
	if len(sketch) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (x.user_id) x.subject_id, x.user_id, us.handle, x.sketch
		FROM fairplay_uploads x JOIN users us ON us.id = x.user_id
		WHERE x.subject_kind = $1 AND x.task_slug = $2 AND x.day IS NOT DISTINCT FROM $3::date AND x.user_id <> $4 AND cardinality(x.sketch) > 0
		ORDER BY x.user_id, x.created_at DESC LIMIT 400`, u.Kind, u.TaskSlug, u.Day, u.UserID)
	if err != nil {
		return err
	}
	type cand struct {
		id, user, handle string
		sketch           []int64
	}
	var cands []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.id, &c.user, &c.handle, &c.sketch); err != nil {
			rows.Close()
			return err
		}
		cands = append(cands, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	all := [][]int64{sketch}
	for _, c := range cands {
		all = append(all, c.sketch)
	}
	common := commonShingles(all)
	var me string
	if err := tx.QueryRow(ctx, `SELECT handle FROM users WHERE id = $1`, u.UserID).Scan(&me); err != nil {
		return err
	}
	for _, c := range cands {
		j, ok := jaccard(sketch, c.sketch, common)
		if !ok || j < dupThreshold {
			continue
		}
		if err := f.raise(ctx, u.ID, u.UserID, "near_duplicate", j, map[string]any{"other": c.handle, "other_id": c.id, "similarity": round2(j)}); err != nil {
			return err
		}
		// The earlier upload is flagged too: a human decides who copied whom.
		if err := f.raise(ctx, c.id, c.user, "near_duplicate", j, map[string]any{"other": me, "other_id": u.ID, "similarity": round2(j)}); err != nil {
			return err
		}
	}
	return nil
}

func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }

func handles(ctx context.Context, tx pgx.Tx, q string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

type flagger struct {
	tx   pgx.Tx
	kind string
}

// raise stores a flag; a repeat for the same subject and signal refreshes it while it is still open.
func (f flagger) raise(ctx context.Context, subjectID, userID, signal string, score float64, detail map[string]any) error {
	raw, _ := json.Marshal(detail)
	_, err := f.tx.Exec(ctx, `
		INSERT INTO fairplay_flags (id, subject_kind, subject_id, user_id, signal, score, detail) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (subject_kind, subject_id, signal) DO UPDATE SET score = excluded.score, detail = excluded.detail WHERE fairplay_flags.status = 'open'`,
		idgen.New("ff"), f.kind, subjectID, userID, signal, score, raw)
	return err
}
