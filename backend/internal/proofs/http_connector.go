package proofs

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
)

const maxLongPoll = 25 * time.Second

type nextTaskResponse struct {
	ProofID string `json:"proof_id"`
	Task    *Task  `json:"task"`
}

func RegisterConnectorRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/connector/tasks/next", func(w http.ResponseWriter, r *http.Request) {
		agentID := identity.MustFromContext(r.Context()).AgentID
		wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
		deadline := time.Now().Add(min(time.Duration(wait)*time.Second, maxLongPoll))
		for {
			p, t, err := s.Claim(r.Context(), agentID)
			if err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			if p != nil {
				httpx.Respond(w, http.StatusOK, nextTaskResponse{ProofID: p.ID, Task: t})
				return
			}
			if time.Now().After(deadline) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(time.Second):
			}
		}
	})
	mux.HandleFunc("GET /api/v1/connector/proofs/{id}/repo.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		tar, err := s.RepoTar(r.Context(), identity.MustFromContext(r.Context()).AgentID, r.PathValue("id"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(tar)))
		_, _ = w.Write(tar)
	})
	mux.HandleFunc("POST /api/v1/connector/proofs/{id}/started", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Started(r.Context(), identity.MustFromContext(r.Context()).AgentID, r.PathValue("id")); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/connector/proofs/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		agentID, proofID := identity.MustFromContext(r.Context()).AgentID, r.PathValue("id")
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxResultBodyBytes))
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &tooBig):
			// Too big to even read: the verdict is known without the body.
			if err := s.FailOversized(r.Context(), agentID, proofID); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			httpx.WriteError(w, r, errDiffTooLarge)
			return
		case err != nil:
			httpx.WriteError(w, r, httpx.InvalidBody("could not read the request body"))
			return
		}
		var in ResultInput
		if err := httpx.Decode(raw, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if err := s.SubmitResult(r.Context(), agentID, proofID, in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
