package products

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

// RegisterPublicRoutes mounts the routes anyone can call; the viewer (if signed in) only personalises them.
func RegisterPublicRoutes(mux *http.ServeMux, s *Service, viewer func(r *http.Request) string) {
	mux.HandleFunc("GET /api/v1/products", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.List(r.Context())
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
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
		res, err := s.Results(r.Context(), r.PathValue("slug"), viewer(r))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/product-entries/{id}/site/{path...}", serveSite(s, viewer))
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

// RegisterOwnerRoutes mounts the session-protected routes.
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
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
	mux.HandleFunc("POST /api/v1/product-entries/{id}/vote", func(w http.ResponseWriter, r *http.Request) {
		e, err := s.Vote(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, e)
	})
}
