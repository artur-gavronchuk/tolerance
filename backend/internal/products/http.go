package products

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

// The results list is cut to ?limit= entries (Results.Total says how many there are).
const (
	defaultResultsLimit = 40
	maxResultsLimit     = 500
)

// RegisterPublicRoutes mounts the routes anyone can call; the viewer (if signed in) only personalises them.
func RegisterPublicRoutes(mux *http.ServeMux, s *Service, viewer func(r *http.Request) string) {
	mux.HandleFunc("GET /api/v1/products", func(w http.ResponseWriter, r *http.Request) {
		l, err := s.List(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, l)
	})
	mux.HandleFunc("GET /api/v1/products/{slug}", func(w http.ResponseWriter, r *http.Request) {
		d, err := s.Get(r.Context(), r.PathValue("slug"), viewer(r))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, d)
	})
	mux.HandleFunc("GET /api/v1/products/{slug}/results", func(w http.ResponseWriter, r *http.Request) {
		limit := defaultResultsLimit
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				httpx.WriteError(w, r, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed", "limit must be a positive integer", "limit", "invalid"))
				return
			}
			limit = min(n, maxResultsLimit)
		}
		res, err := s.results(r.Context(), r.PathValue("slug"), viewer(r), limit)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if len(res.Entries) > limit { // the standings are ranked in full; only the page that goes out is cut
			res.Entries = res.Entries[:limit]
		}
		httpx.Respond(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/product-entries/{id}/site/{path...}", serveSite(s, viewer))
	mux.HandleFunc("GET /api/v1/product-entries/{id}/source", func(w http.ResponseWriter, r *http.Request) {
		files, err := s.Source(r.Context(), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"files": files})
	})
	mux.HandleFunc("GET /api/v1/product-entries/{id}/zip", func(w http.ResponseWriter, r *http.Request) {
		data, err := s.Zip(r.Context(), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("id")+`.zip"`)
		_, _ = w.Write(data)
	})
}

// siteCSP serves uploaded sites as an opaque origin: their scripts run but can't read the session cookie
// or call the API as the viewer (cmd/api also refuses non-GET requests with Origin: null).
const siteCSP = "sandbox allow-scripts allow-forms allow-popups allow-modals"

func serveSite(s *Service, viewer func(r *http.Request) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", siteCSP)
		files, err := s.SiteFiles(r.Context(), r.PathValue("id"), viewer(r))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		raw := r.PathValue("path")
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
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		ct := mime.TypeByExtension(path.Ext(p))
		if ct == "" {
			ct = http.DetectContentType(body)
		}
		w.Header().Set("Content-Type", ct)
		_, _ = w.Write(body)
	}
}

// CanAdmin reports whether the signed-in caller may close and reopen tasks: an admin, or anyone on a local
// run with dev login on.
func CanAdmin(devLogin bool, role string) bool { return identity.CanAdmin(devLogin, role) }

// RegisterOwnerRoutes mounts the session-protected routes.
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service, devLogin bool) {
	// adminTask runs a close/reopen action for an admin and answers with the task as the admin sees it.
	adminTask := func(act func(r *http.Request, uid, slug string) error) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			a := identity.MustFromContext(r.Context())
			if !CanAdmin(devLogin, a.Role) {
				httpx.WriteError(w, r, httpx.Forbidden("Admin role required"))
				return
			}
			slug := r.PathValue("slug")
			if err := act(r, a.UserID, slug); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			d, err := s.Get(r.Context(), slug, a.UserID)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			httpx.Respond(w, http.StatusOK, d)
		}
	}
	// POST /products/start-next: end uploads of the open task and start the next one now (the Monday rotation, early).
	mux.HandleFunc("POST /api/v1/products/start-next", func(w http.ResponseWriter, r *http.Request) {
		a := identity.MustFromContext(r.Context())
		if !CanAdmin(devLogin, a.Role) {
			httpx.WriteError(w, r, httpx.Forbidden("Admin role required"))
			return
		}
		slug, err := s.StartNext(r.Context(), a.UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		d, err := s.Get(r.Context(), slug, a.UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, d)
	})
	// POST /products/{slug}/close {"final": bool}: end uploads now (and, with final, the voting window too).
	mux.HandleFunc("POST /api/v1/products/{slug}/close", adminTask(func(r *http.Request, uid, slug string) error {
		var body struct {
			Final bool `json:"final"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body)
		return s.Close(r.Context(), uid, slug, body.Final)
	}))
	// POST /products/{slug}/reopen {"days": n}: uploads open again for n days (default 7).
	mux.HandleFunc("POST /api/v1/products/{slug}/reopen", adminTask(func(r *http.Request, uid, slug string) error {
		body := struct {
			Days int `json:"days"`
		}{Days: 7}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body)
		return s.Reopen(r.Context(), uid, slug, body.Days)
	}))
	mux.HandleFunc("POST /api/v1/products/{slug}/entries", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes+(256<<10))
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				httpx.WriteError(w, r, httpx.New(http.StatusRequestEntityTooLarge, "body_too_large", "The upload is larger than 5 MB"))
				return
			}
			httpx.WriteError(w, r, invalid("Send a multipart form with a file field"))
			return
		}
		defer r.MultipartForm.RemoveAll()
		f, hdr, err := r.FormFile("file")
		if err != nil {
			httpx.WriteError(w, r, invalid("The file field is required"))
			return
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, MaxUploadBytes+1))
		if err != nil || len(data) > MaxUploadBytes {
			httpx.WriteError(w, r, httpx.New(http.StatusRequestEntityTooLarge, "body_too_large", "The upload is larger than 5 MB"))
			return
		}
		e, err := s.Create(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("slug"), hdr.Filename, data, r.FormValue("made_with"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, e)
	})
	// GET /products/{slug}/compare/next: the next anonymous pair of sites for the caller to judge.
	mux.HandleFunc("GET /api/v1/products/{slug}/compare/next", func(w http.ResponseWriter, r *http.Request) {
		n, err := s.Next(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("slug"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, n)
	})
	// POST /products/{slug}/compare {"a", "b", "winner"}: one judgment per person per pair.
	mux.HandleFunc("POST /api/v1/products/{slug}/compare", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<10))
		var body struct {
			A      string `json:"a"`
			B      string `json:"b"`
			Winner string `json:"winner"`
		}
		if err == nil {
			err = httpx.Decode(raw, &body)
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		j, err := s.Judge(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("slug"), body.A, body.B, body.Winner)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, j)
	})
	mux.HandleFunc("POST /api/v1/product-entries/{id}/vote", func(w http.ResponseWriter, r *http.Request) {
		e, err := s.Vote(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, e)
	})
	mux.HandleFunc("DELETE /api/v1/product-entries/{id}/vote", func(w http.ResponseWriter, r *http.Request) {
		e, err := s.Unvote(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, e)
	})
}
