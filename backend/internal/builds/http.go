package builds

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/limits"
	"tolerance/internal/platform/ratelimit"
)

// siteCSP serves uploaded sites as an opaque origin: their scripts run but can't read the session cookie or
// call the API as the viewer. frame-ancestors lets the build page embed them (and overrides the proxy's
// X-Frame-Options: DENY); it names the site's origin because 'self' never matches a sandboxed document.
func siteCSP(publicURL string) string {
	ancestors := "'self'"
	if u, err := url.Parse(publicURL); err == nil && u.Scheme != "" && u.Host != "" {
		ancestors = u.Scheme + "://" + u.Host
	}
	return "sandbox allow-scripts allow-forms allow-popups allow-modals; frame-ancestors " + ancestors
}

// RegisterPublicRoutes mounts the reads; viewer personalises them when someone is signed in.
func RegisterPublicRoutes(mux *http.ServeMux, s *Service, viewer func(r *http.Request) string, publicURL string) {
	csp := siteCSP(publicURL)
	mux.HandleFunc("GET /api/v1/builds", func(w http.ResponseWriter, r *http.Request) {
		cards, err := s.List(r.Context(), viewer(r))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": cards})
	})
	mux.HandleFunc("GET /api/v1/builds/{slug}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Page(r.Context(), r.PathValue("slug"), viewer(r))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
	mux.HandleFunc("GET /api/v1/build-entries/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.EntryPage(r.Context(), r.PathValue("id"), viewer(r))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
	mux.HandleFunc("GET /api/v1/builds/{slug}/task.zip", func(w http.ResponseWriter, r *http.Request) {
		data, err := s.TaskZip(r.PathValue("slug"), publicURL)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("slug")+`.zip"`)
		_, _ = w.Write(data)
	})
	mux.HandleFunc("GET /api/v1/build-entries/{id}/shot.jpg", func(w http.ResponseWriter, r *http.Request) {
		shot, err := s.Shot(r.Context(), r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=600") // the page adds ?v=<version>
		_, _ = w.Write(shot)
	})
	mux.HandleFunc("GET /api/v1/build-entries/{id}/site/{path...}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		p, body, err := s.SiteFile(r.Context(), r.PathValue("id"), r.PathValue("path"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		ct := mime.TypeByExtension(path.Ext(p))
		if ct == "" {
			ct = http.DetectContentType(body)
		}
		if strings.HasPrefix(ct, "text/html") {
			body = withStorageShim(body)
		}
		w.Header().Set("Content-Type", ct)
		_, _ = w.Write(body)
	})
}

// storageShim swaps localStorage/sessionStorage for in-memory ones when the browser refuses them, as it does
// in the sandboxed (opaque-origin) preview: games that keep a best score would otherwise crash there.
const storageShim = `<script>(function(){function mk(){var d={};return{getItem:function(k){return Object.prototype.hasOwnProperty.call(d,k)?d[k]:null},` +
	`setItem:function(k,v){d[k]=String(v)},removeItem:function(k){delete d[k]},clear:function(){d={}},key:function(i){return Object.keys(d)[i]||null},` +
	`get length(){return Object.keys(d).length}}}["localStorage","sessionStorage"].forEach(function(n){try{window[n].length}catch(e){` +
	`try{Object.defineProperty(window,n,{value:mk(),configurable:true})}catch(e2){}}})})()</script>`

// withStorageShim puts storageShim before the page's own scripts: right after <head>, else at the very start.
func withStorageShim(body []byte) []byte {
	lower := bytes.ToLower(body[:min(len(body), 4096)])
	at := 0
	if i := bytes.Index(lower, []byte("<head")); i >= 0 {
		if j := bytes.IndexByte(lower[i:], '>'); j >= 0 {
			at = i + j + 1
		}
	}
	out := make([]byte, 0, len(body)+len(storageShim))
	out = append(out, body[:at]...)
	out = append(out, storageShim...)
	return append(out, body[at:]...)
}

// RegisterOwnerRoutes mounts the session-protected writes.
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service, limiter *ratelimit.Limiter, devLogin bool) {
	mux.HandleFunc("POST /api/v1/builds/{slug}/entries", func(w http.ResponseWriter, r *http.Request) {
		uid := identity.MustFromContext(r.Context()).UserID
		if !limiter.Allow("build-upload:"+uid, limits.Cap(30), time.Hour) {
			w.Header().Set("Retry-After", strconv.Itoa(600))
			httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many uploads this hour, try again later"))
			return
		}
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
		e, err := s.Create(r.Context(), uid, r.PathValue("slug"), hdr.Filename, data, r.FormValue("made_with"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, e)
	})
	vote := func(on bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			e, err := s.Vote(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"), on)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			httpx.Respond(w, http.StatusOK, e)
		}
	}
	mux.HandleFunc("POST /api/v1/build-entries/{id}/vote", vote(true))
	mux.HandleFunc("DELETE /api/v1/build-entries/{id}/vote", vote(false))
	mux.HandleFunc("DELETE /api/v1/build-entries/{id}", func(w http.ResponseWriter, r *http.Request) {
		a := identity.MustFromContext(r.Context())
		if err := s.Delete(r.Context(), a.UserID, identity.CanAdmin(devLogin, a.Role), r.PathValue("id")); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
