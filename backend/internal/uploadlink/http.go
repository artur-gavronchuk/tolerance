package uploadlink

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
	"tolerance/internal/analytics"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/daily"
	"tolerance/internal/games"
	"tolerance/internal/identity"
	"tolerance/internal/platform/clientip"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/products"
	"tolerance/internal/submissions"
	"tolerance/internal/tasks"
)

// maxBotArchiveBytes bounds a raw tanks upload body (botpkg.Normalize bounds the archive itself further).
const maxBotArchiveBytes = 2 << 20

// Prefix is the URL prefix every token route lives under; the request logger redacts the segment after it.
const Prefix = "/api/v1/u/"

// RedactPath hides the token segment of a token route (it is a credential and must never reach a log).
func RedactPath(p string) string {
	rest, ok := cutPrefix(p, Prefix)
	if !ok {
		return p
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			return Prefix + "{token}" + rest[i:]
		}
	}
	return Prefix + "{token}"
}

func cutPrefix(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return s, false
}

// RegisterOwnerRoutes mounts the session-protected link management routes.
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/me/upload-link", func(w http.ResponseWriter, r *http.Request) {
		st, err := s.Status(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, st)
	})
	// POST creates the link or rotates it; the token is in this response only.
	mux.HandleFunc("POST /api/v1/me/upload-link", func(w http.ResponseWriter, r *http.Request) {
		token, st, err := s.Rotate(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, map[string]any{"token": token, "active": st.Active, "created_at": st.CreatedAt, "last_used_at": st.LastUsedAt})
	})
	mux.HandleFunc("DELETE /api/v1/me/upload-link", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Revoke(r.Context(), identity.MustFromContext(r.Context()).UserID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, Status{})
	})
}

type Deps struct {
	Links       *Service
	Pool        *db.Pool
	Daily       *daily.Service
	Submissions *submissions.Service
	Products    *products.Service
	Games       *games.Service
	Limiter     *ratelimit.Limiter
	TrustProxy  bool
}

// RegisterPublicRoutes mounts /api/v1/u/{token}/…: the same actions as the session routes for daily
// submissions, product entries and tanks bots, authenticated by the token alone.
func RegisterPublicRoutes(mux *http.ServeMux, d Deps) {
	// route wraps h: it resolves the token to a user (401 on a bad or revoked token) and rate-limits by token.
	route := func(pattern string, perMinute int, h func(w http.ResponseWriter, r *http.Request, uid string)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			token := r.PathValue("token")
			if !d.Limiter.Allow("ul:ip:"+clientip.FromRequest(r, d.TrustProxy), 300, time.Minute) {
				httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many requests, slow down"))
				return
			}
			uid, err := d.Links.UserByToken(r.Context(), token)
			if errors.Is(err, ErrNoLink) {
				httpx.WriteError(w, r, httpx.Unauthenticated("This upload link is not valid (revoked or rotated)"))
				return
			}
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			if !d.Limiter.Allow("ul:"+Key(token), perMinute, time.Minute) {
				w.Header().Set("Retry-After", "60")
				httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many requests on this upload link, wait a minute"))
				return
			}
			if r.Method == http.MethodPost {
				analytics.Track("upload_link.used", uid, nil)
			}
			h(w, r, uid)
		})
	}
	const read, write = 120, 12

	// --- daily ---
	route("GET "+Prefix+"{token}/daily", read, func(w http.ResponseWriter, r *http.Request, uid string) {
		out, err := d.Daily.Get(r.Context(), daily.Today(), uid, d.Submissions.MyDay)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, out)
	})
	route("GET "+Prefix+"{token}/daily/repo.zip", read, func(w http.ResponseWriter, r *http.Request, uid string) {
		slug, err := d.Daily.TaskFor(r.Context(), daily.Today())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var tarball []byte
		var taskMD string
		err = d.Pool.Tx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT repo_tar, task_md FROM tasks WHERE slug = $1`, slug).Scan(&tarball, &taskMD)
		})
		var z []byte
		if err == nil {
			z, err = tasks.RepoZip(tarball, taskMD)
		}
		if err == nil {
			analytics.Track("daily.download", uid, nil)
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+slug+`.zip"`)
		_, _ = w.Write(z)
	})
	route("POST "+Prefix+"{token}/daily", write, func(w http.ResponseWriter, r *http.Request, uid string) {
		name, data, ok := readUpload(w, r, submissions.MaxUploadBytes)
		if !ok {
			return
		}
		sub, err := d.Submissions.Create(r.Context(), uid, "", name, data, r.FormValue("made_with"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, map[string]any{"submission": sub, "status_url": Prefix + r.PathValue("token") + "/submissions/" + sub.ID})
	})
	route("GET "+Prefix+"{token}/submissions/{id}", read, func(w http.ResponseWriter, r *http.Request, uid string) {
		sub, err := d.Submissions.Get(r.Context(), uid, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, sub)
	})

	// --- products ---
	route("GET "+Prefix+"{token}/products/{slug}", read, func(w http.ResponseWriter, r *http.Request, uid string) {
		out, err := d.Products.Get(r.Context(), r.PathValue("slug"), uid)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, out)
	})
	route("POST "+Prefix+"{token}/products/{slug}", write, func(w http.ResponseWriter, r *http.Request, uid string) {
		name, data, ok := readUpload(w, r, products.MaxUploadBytes)
		if !ok {
			return
		}
		e, err := d.Products.Create(r.Context(), uid, r.PathValue("slug"), name, data, r.FormValue("made_with"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, map[string]any{"entry": e,
			"status_url": Prefix + r.PathValue("token") + "/products/" + e.TaskSlug + "/entries/" + e.ID})
	})
	route("GET "+Prefix+"{token}/products/{slug}/entries/{id}", read, func(w http.ResponseWriter, r *http.Request, uid string) {
		det, err := d.Products.Get(r.Context(), r.PathValue("slug"), uid)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		for _, e := range det.Mine {
			if e.ID == r.PathValue("id") {
				httpx.Respond(w, http.StatusOK, e)
				return
			}
		}
		httpx.WriteError(w, r, httpx.NotFound())
	})

	// --- tanks ---
	route("GET "+Prefix+"{token}/tanks", read, func(w http.ResponseWriter, r *http.Request, uid string) {
		out, err := d.Games.MyTanks(r.Context(), uid)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, out)
	})
	// The body is the raw zip or tar.gz of the bot (not multipart).
	route("POST "+Prefix+"{token}/tanks", write, func(w http.ResponseWriter, r *http.Request, uid string) {
		raw, err := httpx.ReadBodyLimit(w, r, maxBotArchiveBytes)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		v, err := d.Games.UploadVersion(r.Context(), uid, raw)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, map[string]any{"version": v, "status_url": Prefix + r.PathValue("token") + "/tanks/versions/" + v.ID})
	})
	// A version with its checks, plus the bot's latest matches; the check match and every match have a report.
	route("GET "+Prefix+"{token}/tanks/versions/{id}", read, func(w http.ResponseWriter, r *http.Request, uid string) {
		out, err := d.Games.MyTanks(r.Context(), uid)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		base := Prefix + r.PathValue("token") + "/tanks/matches/"
		for _, v := range out.Versions {
			if v.ID != r.PathValue("id") {
				continue
			}
			type match struct {
				ID        string `json:"id"`
				Kind      string `json:"kind"`
				Status    string `json:"status"`
				ReportURL string `json:"report_url"`
			}
			ms := []match{}
			for _, m := range out.Matches {
				ms = append(ms, match{ID: m.ID, Kind: m.Kind, Status: m.Status, ReportURL: base + m.ID + "/report"})
			}
			var checkReport *string
			if v.CheckMatchID != nil {
				u := base + *v.CheckMatchID + "/report"
				checkReport = &u
			}
			httpx.Respond(w, http.StatusOK, map[string]any{"version": v, "check_match_report_url": checkReport, "bot": out.Bot, "latest_matches": ms})
			return
		}
		httpx.WriteError(w, r, httpx.NotFound())
	})
	route("GET "+Prefix+"{token}/tanks/matches/{id}/report", read, func(w http.ResponseWriter, r *http.Request, uid string) {
		text, err := d.Games.MatchReport(r.Context(), uid, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(text))
	})
}

// readUpload parses a multipart form with a "file" field (a zip or a patch) bounded by max bytes.
func readUpload(w http.ResponseWriter, r *http.Request, max int) (name string, data []byte, ok bool) {
	tooBig := httpx.New(http.StatusRequestEntityTooLarge, "body_too_large", "The upload is larger than 5 MB")
	r.Body = http.MaxBytesReader(w, r.Body, int64(max)+(256<<10))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			httpx.WriteError(w, r, tooBig)
		} else {
			httpx.WriteError(w, r, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed", "Send a multipart form with a file field (curl -F file=@result.zip)", "file", "invalid"))
		}
		return "", nil, false
	}
	defer r.MultipartForm.RemoveAll()
	f, hdr, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed", "The file field is required", "file", "required"))
		return "", nil, false
	}
	defer f.Close()
	data, err = io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil || len(data) > max {
		httpx.WriteError(w, r, tooBig)
		return "", nil, false
	}
	return hdr.Filename, data, true
}
