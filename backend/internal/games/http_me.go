package games

import (
	"encoding/base64"
	"net/http"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

// maxVersionUploadBytes bounds a version upload's raw (still base64-encoded) request body; the decoded
// archive itself is further bounded by botpkg.Normalize's own 1 MiB compressed / 4 MiB unpacked limits.
const maxVersionUploadBytes = 2 << 20

type botInput struct {
	Name string `json:"name"`
}

type versionInput struct {
	ArchiveBase64 string `json:"archive_base64"`
}

// decodeArchiveBody reads a {archive_base64} request body and base64-decodes it; bad JSON is the usual
// invalid_body, but a body that isn't valid base64 is reported as validation_failed on fields.archive_base64
// specifically, since the field decoded fine as JSON but its content is what's wrong.
func decodeArchiveBody(raw []byte) ([]byte, error) {
	var in versionInput
	if err := httpx.Decode(raw, &in); err != nil {
		return nil, err
	}
	archive, err := base64.StdEncoding.DecodeString(in.ArchiveBase64)
	if err != nil {
		return nil, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed",
			"archive_base64 must be valid base64", "archive_base64", "invalid")
	}
	return archive, nil
}

// RegisterOwnerRoutes registers the tanks arena's owner routes: the caller's own /tanks/me page, creating
// or renaming their bot, uploading a version, starting an agent run, and reading their own match log.
func RegisterOwnerRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/me/tanks", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.MyTanks(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, out)
	})

	mux.HandleFunc("POST /api/v1/me/tanks/bot", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var in botInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		m, err := s.SaveBot(r.Context(), identity.MustFromContext(r.Context()).UserID, in.Name)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, m)
	})

	mux.HandleFunc("POST /api/v1/me/tanks/versions", func(w http.ResponseWriter, r *http.Request) {
		raw, err := httpx.ReadBodyLimit(w, r, maxVersionUploadBytes)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		archive, err := decodeArchiveBody(raw)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		v, err := s.UploadVersion(r.Context(), identity.MustFromContext(r.Context()).UserID, archive)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, v)
	})

	mux.HandleFunc("POST /api/v1/me/tanks/agent-runs", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.StartAgentRun(r.Context(), identity.MustFromContext(r.Context()).UserID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusCreated, p)
	})

	mux.HandleFunc("GET /api/v1/me/tanks/matches/{id}/log", func(w http.ResponseWriter, r *http.Request) {
		l, err := s.MatchLog(r.Context(), identity.MustFromContext(r.Context()).UserID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, l)
	})
}
