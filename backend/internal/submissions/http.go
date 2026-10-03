package submissions

import (
	"errors"
	"io"
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

// RegisterOwnerRoutes mounts the session-protected submission routes.
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("POST /api/v1/submissions", func(w http.ResponseWriter, r *http.Request) {
		// 5 MiB for the file plus headroom for the multipart framing and the small text fields.
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
		if hdr.Size > MaxUploadBytes {
			httpx.WriteError(w, r, httpx.New(http.StatusRequestEntityTooLarge, "body_too_large", "The upload is larger than 5 MB"))
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, MaxUploadBytes+1))
		if err != nil || len(data) > MaxUploadBytes {
			httpx.WriteError(w, r, httpx.New(http.StatusRequestEntityTooLarge, "body_too_large", "The upload is larger than 5 MB"))
			return
		}
		sub, err := s.Create(r.Context(), identity.MustFromContext(r.Context()).UserID, r.FormValue("task_slug"), hdr.Filename, data, r.FormValue("made_with"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, sub)
	})

	mux.HandleFunc("GET /api/v1/submissions/{id}", func(w http.ResponseWriter, r *http.Request) {
		sub, err := s.Get(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, sub)
	})
}
